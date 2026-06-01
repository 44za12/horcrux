<script lang="ts">
  import { onMount } from 'svelte'
  import { ListFiles, AddFile, GetFile, RemoveFile } from '../../wailsjs/go/main/App'

  let files: any[] = []
  let search = ''
  let loading = true
  let showAdd = false
  let addError = ''
  let adding = false
  let selectedFile: File | null = null
  let fileInputEl: any = null

  $: filtered = search
    ? files.filter((f: any) =>
        f.Name.toLowerCase().includes(search.toLowerCase()) ||
        f.MimeType.toLowerCase().includes(search.toLowerCase())
      )
    : files

  let searchEl: any = null

  export function triggerAdd() { showAdd = true }
  export function focusSearch() { searchEl && searchEl.focus() }

  async function load() {
    try { files = await ListFiles() || [] } catch (e: any) { console.error(e) }
    loading = false
  }

  function formatSize(bytes: number): string {
    if (bytes < 1024) return bytes + ' B'
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
    return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
  }

  function fileIcon(mimeType: string): string {
    if (mimeType.startsWith('image/')) return '&#x1F5BC;'
    if (mimeType.startsWith('video/')) return '&#x1F3AC;'
    if (mimeType.startsWith('audio/')) return '&#x1F3B5;'
    if (mimeType.includes('pdf')) return '&#x1F4D1;'
    if (mimeType.includes('zip') || mimeType.includes('tar') || mimeType.includes('gz')) return '&#x1F4E6;'
    if (mimeType.includes('json')) return '&#x1F4CB;'
    if (mimeType.includes('text') || mimeType.includes('document')) return '&#x1F4C4;'
    return '&#x1F4C1;'
  }

  function handleFileSelect(e: any) {
    const target = e.target as HTMLInputElement
    if (target.files && target.files.length > 0) {
      selectedFile = target.files[0]
    }
  }

  async function upload() {
    if (!selectedFile) { addError = 'Select a file to upload'; return }
    adding = true
    addError = ''
    try {
      const reader = new FileReader()
      const content = await new Promise<string>((resolve, reject) => {
        reader.onload = () => {
          const arr = new Uint8Array(reader.result as ArrayBuffer)
          let binary = ''
          for (let i = 0; i < arr.length; i++) {
            binary += String.fromCharCode(arr[i])
          }
          resolve(btoa(binary))
        }
        reader.onerror = reject
        reader.readAsArrayBuffer(selectedFile)
      })
      await AddFile(selectedFile.name, selectedFile.type || 'application/octet-stream', content)
      showAdd = false
      selectedFile = null
      addError = ''
      await load()
    } catch (e: any) {
      addError = e.toString().replace('Error: ', '')
    }
    adding = false
  }

  async function download(name: string) {
    try {
      const result = await GetFile(name)
      const parsed = JSON.parse(result)
      const binary = atob(parsed.Data)
      const bytes = new Uint8Array(binary.length)
      for (let i = 0; i < binary.length; i++) {
        bytes[i] = binary.charCodeAt(i)
      }
      const blob = new Blob([bytes], { type: parsed.MimeType })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = name
      a.click()
      URL.revokeObjectURL(url)
    } catch (e) { console.error(e) }
  }

  async function remove(name: string) {
    try { await RemoveFile(name); await load() } catch (e) { console.error(e) }
  }

  onMount(load)
</script>

<div class="slide-up">
  <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:14px;">
    <div style="display:flex; align-items:center; gap:10px;">
      <div class="search-wrap">
        <input class="search-input" type="search" placeholder="Search files..." bind:value={search} bind:this={searchEl} />
      </div>
      {#if search}
        <span style="font-size:12px; color:var(--text-tertiary);">{filtered.length} results</span>
      {:else}
        <span style="font-size:12px; color:var(--text-tertiary);">{files.length} file{files.length !== 1 ? 's' : ''}</span>
      {/if}
    </div>
    <div style="display:flex; gap:6px;">
      <button class="btn btn-primary btn-sm" on:click={() => showAdd = true}>+ Upload File</button>
    </div>
  </div>

  {#if loading}
    <div class="empty">
      <div class="empty-icon">&#x23F3;</div>
      <div class="empty-title">Loading...</div>
    </div>
  {:else if filtered.length === 0}
    <div class="empty">
      <div class="empty-icon">&#x1F4C1;</div>
      <div class="empty-title">{search ? 'No matches' : 'No files yet'}</div>
      <div class="empty-desc">{search ? 'Try a different search term' : 'Click "+ Upload File" to securely store a file'}</div>
    </div>
  {:else}
    <div class="card" style="padding:0; overflow:hidden;">
      <div class="list-header" style="grid-template-columns: 2fr 1fr 80px 100px;">
        <span>Name</span>
        <span>Type</span>
        <span>Size</span>
        <span></span>
      </div>
      {#each filtered as entry}
        <div class="list-row" style="grid-template-columns: 2fr 1fr 80px 100px;">
          <div class="list-cell" style="font-weight:600;">
            <span style="margin-right:6px;">{@html fileIcon(entry.MimeType)}</span>
            {entry.Name}
          </div>
          <div class="list-cell" style="color:var(--text-secondary); font-size:12px;">{entry.MimeType}</div>
          <div class="list-cell" style="color:var(--text-tertiary); font-size:12px;">{formatSize(entry.Size)}</div>
          <div class="list-actions">
            <button class="btn-icon" on:click={() => download(entry.Name)} title="Download">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
            </button>
            <button class="btn-icon btn-danger" on:click={() => remove(entry.Name)} title="Delete">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6m3 0V4a2 2 0 012-2h4a2 2 0 012 2v2"/></svg>
            </button>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>

{#if showAdd}
  <div class="modal-backdrop" on:click|self={() => { showAdd = false; addError = ''; selectedFile = null }}>
    <div class="modal" on:click|stopPropagation>
      <div class="modal-header">Upload File</div>
      <div class="modal-body">
        {#if addError}<div class="modal-error">{addError}</div>{/if}
        <div style="border:2px dashed var(--border); border-radius:var(--radius); padding:24px; text-align:center; cursor:pointer; position:relative;"
          on:click={() => fileInputEl && fileInputEl.click()}>
          <input type="file" style="display:none;" bind:this={fileInputEl} on:change={handleFileSelect} />
          {#if selectedFile}
            <div style="font-weight:600; margin-bottom:4px;">{selectedFile.name}</div>
            <div style="font-size:12px; color:var(--text-tertiary);">{formatSize(selectedFile.size)} &middot; {selectedFile.type || 'unknown'}</div>
          {:else}
            <div style="font-size:28px; margin-bottom:8px;">&#x1F4E4;</div>
            <div style="font-weight:500; color:var(--text-secondary);">Click to select a file</div>
            <div style="font-size:11px; color:var(--text-tertiary); margin-top:4px;">File will be encrypted before storage</div>
          {/if}
        </div>
      </div>
      <div class="modal-footer">
        <button class="btn" on:click={() => { showAdd = false; addError = ''; selectedFile = null }} disabled={adding}>Cancel</button>
        <button class="btn btn-primary" on:click={upload} disabled={!selectedFile || adding}>
          {adding ? 'Encrypting...' : 'Upload'}
        </button>
      </div>
    </div>
  </div>
{/if}
