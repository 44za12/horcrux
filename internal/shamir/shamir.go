package shamir

import (
	"crypto/rand"
	"fmt"
	"io"

	"horcrux/internal/crypto"
)

func GFMul(a, b byte) byte {
	var p byte
	for i := 0; i < 8; i++ {
		if b&1 != 0 {
			p ^= a
		}
		hi := a & 0x80
		a <<= 1
		if hi != 0 {
			a ^= 0x1B
		}
		b >>= 1
	}
	return p
}

func GFInv(a byte) byte {
	if a == 0 {
		return 0
	}
	a2 := GFMul(a, a)
	a4 := GFMul(a2, a2)
	a8 := GFMul(a4, a4)
	a16 := GFMul(a8, a8)
	a32 := GFMul(a16, a16)
	a64 := GFMul(a32, a32)
	a128 := GFMul(a64, a64)
	result := a2
	result = GFMul(result, a4)
	result = GFMul(result, a8)
	result = GFMul(result, a16)
	result = GFMul(result, a32)
	result = GFMul(result, a64)
	result = GFMul(result, a128)
	return result
}

func GFDiv(a, b byte) byte {
	if a == 0 {
		return 0
	}
	if b == 0 {
		panic("division by zero in GF(256)")
	}
	return GFMul(a, GFInv(b))
}

func Split(secret []byte, n, k int) ([][]byte, error) {
	if k > n {
		return nil, fmt.Errorf("threshold (%d) cannot exceed total shares (%d)", k, n)
	}
	if n > 255 {
		return nil, fmt.Errorf("maximum 255 shares supported")
	}
	if k < 2 {
		return nil, fmt.Errorf("threshold must be at least 2")
	}

	shares := make([][]byte, n)
	for i := range shares {
		shares[i] = make([]byte, 1+len(secret))
		shares[i][0] = byte(i + 1)
	}

	for byteIdx, secretByte := range secret {
		coeffs := make([]byte, k)
		coeffs[0] = secretByte
		if _, err := io.ReadFull(rand.Reader, coeffs[1:]); err != nil {
			return nil, fmt.Errorf("generating random coefficients: %w", err)
		}

		for i := 0; i < n; i++ {
			x := byte(i + 1)
			var y byte
			for j := len(coeffs) - 1; j >= 0; j-- {
				y = GFMul(y, x) ^ coeffs[j]
			}
			shares[i][byteIdx+1] = y
		}
	}

	return shares, nil
}

func Combine(shares [][]byte) ([]byte, error) {
	if len(shares) < 2 {
		return nil, fmt.Errorf("need at least 2 shares to reconstruct")
	}

	secretLen := len(shares[0]) - 1
	result := make([]byte, secretLen)

	xVals := make([]byte, len(shares))
	for i, s := range shares {
		xVals[i] = s[0]
	}

	for idx := 0; idx < secretLen; idx++ {
		var val byte
		for i, xi := range xVals {
			yi := shares[i][idx+1]
			num := byte(1)
			den := byte(1)
			for j, xj := range xVals {
				if i == j {
					continue
				}
				num = GFMul(num, xj)
				den = GFMul(den, xi^xj)
			}
			lagrange := GFDiv(num, den)
			val ^= GFMul(yi, lagrange)
		}
		result[idx] = val
	}

	return result, nil
}

func EncryptShare(share []byte, passphrase string) ([]byte, error) {
	return crypto.EncryptData(share, passphrase)
}

func DecryptShare(data []byte, passphrase string) ([]byte, error) {
	share, err := crypto.DecryptData(data, passphrase)
	if err != nil {
		return nil, fmt.Errorf("decrypting share: %w", err)
	}
	return share, nil
}
