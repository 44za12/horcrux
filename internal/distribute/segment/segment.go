package segment

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/klauspost/reedsolomon"
)

// TargetSize is the approximate plaintext size for each segment.
// Segments are content-addressed and only re-uploaded when they change,
// so this size balances dedup granularity against overhead.
const TargetSize = 16 * 1024 * 1024 // 16 MB

// Segment is a content-addressed block of vault data.
// It contains one or more entries (metadata files or file chunks).
type Segment struct {
	Hash      string         // SHA-256 hex of plaintext (set after packing)
	Plaintext []byte         // serialized entries
	Entries   []SegmentEntry // unpacked form
}

// SegmentEntry is one piece of vault data inside a segment.
type SegmentEntry struct {
	Type uint8  // 0 = metadata file, 1 = file chunk
	Name string // filename (metadata) or "filename:chunkIdx" (chunk)
	Data []byte // raw content

	// Metadata for file chunks (empty for metadata files)
	MimeType  string
	TotalSize int64
	ChunkHash string // SHA-256 hex of this chunk's plaintext
}

// SegmentRef is a lightweight reference to a segment stored on providers.
type SegmentRef struct {
	Hash    string `json:"hash"`     // SHA-256 hex of segment plaintext
	Size    int64  `json:"size"`     // plaintext size in bytes
	ShardsN int    `json:"shards_n"` // total erasure shard count (data + parity)
}

// --- Serialization ---

// serializeEntries packs entries into a binary blob.
func serializeEntries(entries []SegmentEntry) []byte {
	var buf bytes.Buffer

	// Header: number of entries (u16)
	binary.Write(&buf, binary.BigEndian, uint16(len(entries)))

	for _, e := range entries {
		// Type (u8)
		buf.WriteByte(e.Type)

		// Name (u16 length + bytes)
		nameBytes := []byte(e.Name)
		binary.Write(&buf, binary.BigEndian, uint16(len(nameBytes)))
		buf.Write(nameBytes)

		// Data (u32 length + bytes)
		binary.Write(&buf, binary.BigEndian, uint32(len(e.Data)))
		buf.Write(e.Data)

		// Metadata for file chunks
		if e.Type == 1 {
			mimeBytes := []byte(e.MimeType)
			binary.Write(&buf, binary.BigEndian, uint16(len(mimeBytes)))
			buf.Write(mimeBytes)
			binary.Write(&buf, binary.BigEndian, e.TotalSize)
			hashBytes := []byte(e.ChunkHash)
			binary.Write(&buf, binary.BigEndian, uint16(len(hashBytes)))
			buf.Write(hashBytes)
		}
	}

	return buf.Bytes()
}

// deserializeEntries unpacks a binary blob back into entries.
func deserializeEntries(data []byte) ([]SegmentEntry, error) {
	reader := bytes.NewReader(data)

	var numEntries uint16
	if err := binary.Read(reader, binary.BigEndian, &numEntries); err != nil {
		return nil, fmt.Errorf("reading segment header: %w", err)
	}
	if numEntries > 4096 {
		return nil, fmt.Errorf("too many entries in segment: %d", numEntries)
	}

	entries := make([]SegmentEntry, 0, numEntries)
	for i := uint16(0); i < numEntries; i++ {
		var e SegmentEntry

		// Type
		typeByte, err := reader.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("reading entry type: %w", err)
		}
		e.Type = typeByte

		// Name
		var nameLen uint16
		if err := binary.Read(reader, binary.BigEndian, &nameLen); err != nil {
			return nil, fmt.Errorf("reading name length: %w", err)
		}
		if nameLen > 4096 {
			return nil, fmt.Errorf("name too long: %d", nameLen)
		}
		nameBytes := make([]byte, nameLen)
		if _, err := io.ReadFull(reader, nameBytes); err != nil {
			return nil, fmt.Errorf("reading name: %w", err)
		}
		e.Name = string(nameBytes)

		// Data
		var dataLen uint32
		if err := binary.Read(reader, binary.BigEndian, &dataLen); err != nil {
			return nil, fmt.Errorf("reading data length: %w", err)
		}
		if dataLen > uint32(TargetSize) {
			return nil, fmt.Errorf("entry data too large: %d", dataLen)
		}
		e.Data = make([]byte, dataLen)
		if _, err := io.ReadFull(reader, e.Data); err != nil {
			return nil, fmt.Errorf("reading entry data: %w", err)
		}

		// File chunk metadata
		if e.Type == 1 {
			var mimeLen uint16
			if err := binary.Read(reader, binary.BigEndian, &mimeLen); err != nil {
				return nil, fmt.Errorf("reading mime length: %w", err)
			}
			mimeBytes := make([]byte, mimeLen)
			if _, err := io.ReadFull(reader, mimeBytes); err != nil {
				return nil, fmt.Errorf("reading mime type: %w", err)
			}
			e.MimeType = string(mimeBytes)

			if err := binary.Read(reader, binary.BigEndian, &e.TotalSize); err != nil {
				return nil, fmt.Errorf("reading total size: %w", err)
			}

			var hashLen uint16
			if err := binary.Read(reader, binary.BigEndian, &hashLen); err != nil {
				return nil, fmt.Errorf("reading hash length: %w", err)
			}
			hashBytes := make([]byte, hashLen)
			if _, err := io.ReadFull(reader, hashBytes); err != nil {
				return nil, fmt.Errorf("reading chunk hash: %w", err)
			}
			e.ChunkHash = string(hashBytes)
		}

		entries = append(entries, e)
	}

	return entries, nil
}

// --- Content addressing ---

// computeHash returns the SHA-256 hex of plaintext.
func computeHash(plaintext []byte) string {
	h := sha256.Sum256(plaintext)
	return fmt.Sprintf("%x", h)
}

// --- Deterministic encryption (convergent) ---

// encryptWithDEK encrypts plaintext with a deterministic nonce derived from
// the plaintext hash. Same plaintext → same ciphertext → same content address.
func encryptWithDEK(plaintext []byte, dek []byte) ([]byte, error) {
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Deterministic nonce: first 12 bytes of SHA-256(plaintext)
	fullHash := sha256.Sum256(plaintext)
	nonce := make([]byte, gcm.NonceSize())
	copy(nonce, fullHash[:gcm.NonceSize()])

	encrypted := gcm.Seal(nil, nonce, plaintext, nil)
	return append(nonce, encrypted...), nil
}

// decryptWithDEK decrypts ciphertext (format: nonce || AES-GCM ciphertext).
func decryptWithDEK(ciphertext []byte, dek []byte) ([]byte, error) {
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize+gcm.Overhead() {
		return nil, fmt.Errorf("ciphertext too short: %d bytes", len(ciphertext))
	}

	nonce := ciphertext[:nonceSize]
	ct := ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ct, nil)
}

// --- Segment-level operations ---

// Pack creates a Segment from entries.
func Pack(entries []SegmentEntry) *Segment {
	plaintext := serializeEntries(entries)
	return &Segment{
		Hash:      computeHash(plaintext),
		Plaintext: plaintext,
		Entries:   entries,
	}
}

// Unpack deserializes a segment's plaintext into entries.
func (s *Segment) Unpack() error {
	entries, err := deserializeEntries(s.Plaintext)
	if err != nil {
		return err
	}
	s.Entries = entries
	return nil
}

// Encrypt encrypts the segment with the DEK. Returns (ciphertext, error).
func (s *Segment) Encrypt(dek []byte) ([]byte, error) {
	return encryptWithDEK(s.Plaintext, dek)
}

// DecryptAndUnpack decrypts ciphertext with the DEK and unpacks entries.
func DecryptAndUnpack(ciphertext []byte, dek []byte) (*Segment, error) {
	plaintext, err := decryptWithDEK(ciphertext, dek)
	if err != nil {
		return nil, err
	}
	seg := &Segment{
		Hash:      computeHash(plaintext),
		Plaintext: plaintext,
	}
	if err := seg.Unpack(); err != nil {
		return nil, err
	}
	return seg, nil
}

// --- Erasure coding (per segment) ---

// ErasureEncode splits segment ciphertext into N shards (M data + N-M parity).
func ErasureEncodeSegment(ciphertext []byte, dataShards, parityShards int) ([][]byte, error) {
	enc, err := reedsolomon.New(dataShards, parityShards)
	if err != nil {
		return nil, fmt.Errorf("creating erasure encoder: %w", err)
	}

	// Prefix with original length for reconstruction
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, uint64(len(ciphertext)))
	buf.Write(ciphertext)

	shards, err := enc.Split(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("splitting data: %w", err)
	}

	if err := enc.Encode(shards); err != nil {
		return nil, fmt.Errorf("encoding parity: %w", err)
	}

	return shards, nil
}

// ErasureDecodeSegment reconstructs segment ciphertext from M-of-N shards.
func ErasureDecodeSegment(shards [][]byte, dataShards, totalShards int) ([]byte, error) {
	enc, err := reedsolomon.New(dataShards, totalShards-dataShards)
	if err != nil {
		return nil, fmt.Errorf("creating erasure decoder: %w", err)
	}

	if err := enc.Reconstruct(shards); err != nil {
		return nil, fmt.Errorf("reconstructing data: %w", err)
	}

	ok, err := enc.Verify(shards)
	if err != nil {
		return nil, fmt.Errorf("verifying reconstruction: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("reconstruction verification failed")
	}

	var buf bytes.Buffer
	if err := enc.Join(&buf, shards, len(shards[0])*dataShards); err != nil {
		return nil, fmt.Errorf("joining shards: %w", err)
	}

	joined := buf.Bytes()
	if len(joined) < 8 {
		return nil, fmt.Errorf("reconstructed data too short")
	}

	originalLen := binary.BigEndian.Uint64(joined[:8])
	if originalLen > uint64(len(joined)-8) {
		return nil, fmt.Errorf("invalid original length")
	}

	return joined[8 : 8+originalLen], nil
}

// PadToEqualSize pads all shards to the same length (required by reedsolomon).
// The last shard may need padding since reedsolomon.Split requires equal sizes.
func PadToEqualSize(shards [][]byte) {
	maxLen := 0
	for _, s := range shards {
		if len(s) > maxLen {
			maxLen = len(s)
		}
	}
	for i := range shards {
		if len(shards[i]) < maxLen {
			padded := make([]byte, maxLen)
			copy(padded, shards[i])
			shards[i] = padded
		}
	}
}

// GenerateDEK creates a random 32-byte Data Encryption Key.
func GenerateDEK() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generating DEK: %w", err)
	}
	return key, nil
}
