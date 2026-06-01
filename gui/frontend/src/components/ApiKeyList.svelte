<script lang="ts">
  import { onMount } from 'svelte'
  import { ListApiKeys, GetApiKey, AddApiKey, RemoveApiKey, GeneratePassword } from '../../wailsjs/go/main/App'

  let keys: any[] = []
  let search = ''
  let loading = true
  let revealed: Record<string, boolean> = {}
  let showAdd = false
  let addService = ''
  let addName = ''
  let addKey = ''
  let addNotes = ''
  let addError = ''
  let editing = false
  let editOrigService = ''
  let editOrigName = ''

  $: filtered = search
    ? keys.filter((k: any) =>
        k.Service.toLowerCase().includes(search.toLowerCase()) ||
        k.Name.toLowerCase().includes(search.toLowerCase()) ||
        (k.Notes && k.Notes.toLowerCase().includes(search.toLowerCase()))
      )
    : keys

  let searchEl: any = null

  export function triggerAdd() { openAdd() }
  export function focusSearch() { searchEl && searchEl.focus() }

  async function load() {
    try { keys = await ListApiKeys() || [] } catch (e: any) { console.error(e) }
    loading = false
  }

  async function copy(text: string) {
    try { await navigator.clipboard.writeText(text) }
    catch {
      const t = document.createElement('textarea'); t.value = text
      document.body.appendChild(t); t.select(); document.execCommand('copy'); document.body.removeChild(t)
    }
  }

  async function copyEntry(entry: any) {
    const key = await GetApiKey(entry.Service, entry.Name)
    await copy(key)
  }

  async function reveal(entry: any) {
    const revealKey = entry.Service + entry.Name
    entry.Key = await GetApiKey(entry.Service, entry.Name)
    revealed[revealKey] = true
    keys = [...keys]
    setTimeout(() => {
      entry.Key = ''
      revealed = { ...revealed, [revealKey]: false }
      keys = [...keys]
    }, 5000)
  }

  async function generateKey() {
    try { addKey = await GeneratePassword() } catch (e) { console.error(e) }
  }

  function openAdd() {
    editing = false
    addService = ''; addName = ''; addKey = ''; addNotes = ''; addError = ''
    showAdd = true
  }

  async function openEdit(entry: any) {
    editing = true
    editOrigService = entry.Service; editOrigName = entry.Name
    addService = entry.Service; addName = entry.Name; addKey = await GetApiKey(entry.Service, entry.Name); addNotes = entry.Notes || ''; addError = ''
    showAdd = true
  }

  async function save() {
    if (!addService || !addName || !addKey) { addError = 'Service, name, and key are required'; return }
    try {
      if (editing) {
        if (editOrigService !== addService || editOrigName !== addName) {
          await RemoveApiKey(editOrigService, editOrigName)
        }
      }
      await AddApiKey(addService, addName, addKey, addNotes)
      showAdd = false; addService = ''; addName = ''; addKey = ''; addNotes = ''; addError = ''
      editing = false
      await load()
    } catch (e: any) { addError = e.toString().replace('Error: ', '') }
  }

  async function remove(service: string, name: string) {
    try { await RemoveApiKey(service, name); await load() } catch (e) { console.error(e) }
  }

  onMount(load)
</script>

<div class="slide-up">
  <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:14px;">
    <div style="display:flex; align-items:center; gap:10px;">
      <div class="search-wrap">
        <input class="search-input" type="search" placeholder="Search API keys..." bind:value={search} bind:this={searchEl} />
      </div>
      {#if search}
        <span style="font-size:12px; color:var(--text-tertiary);">{filtered.length} results</span>
      {:else}
        <span style="font-size:12px; color:var(--text-tertiary);">{keys.length} key{keys.length !== 1 ? 's' : ''}</span>
      {/if}
    </div>
    <div style="display:flex; gap:6px;">
      <button class="btn btn-primary btn-sm" on:click={openAdd}>+ Add Key</button>
    </div>
  </div>

  {#if loading}
    <div class="empty">
      <div class="empty-icon">&#x23F3;</div>
      <div class="empty-title">Loading...</div>
    </div>
  {:else if filtered.length === 0}
    <div class="empty">
      <div class="empty-icon">&#x1F511;</div>
      <div class="empty-title">{search ? 'No matches' : 'No API keys yet'}</div>
      <div class="empty-desc">{search ? 'Try a different search term' : 'Click "+ Add Key" to store an API key'}</div>
    </div>
  {:else}
    <div class="card" style="padding:0; overflow:hidden;">
      <div class="list-header" style="grid-template-columns: 1.5fr 1fr 2fr 1.5fr 100px;">
        <span>Service</span>
        <span>Name</span>
        <span>Key</span>
        <span>Notes</span>
        <span></span>
      </div>
      {#each filtered as entry}
        <div class="list-row" style="grid-template-columns: 1.5fr 1fr 2fr 1.5fr 100px;">
          <div class="list-cell" style="font-weight:600;">{entry.Service}</div>
          <div class="list-cell" style="color:var(--text-secondary);">{entry.Name}</div>
          <div class="list-cell list-cell-mono">
            {revealed[entry.Service + entry.Name] ? entry.Key : '••••••••••••••••'}
          </div>
          <div class="list-cell" style="color:var(--text-tertiary); font-size:12px;">
            {entry.Notes || ''}
          </div>
          <div class="list-actions">
            <button class="btn-icon" on:click={() => reveal(entry)} title="Reveal">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg>
            </button>
            <button class="btn-icon" on:click={() => copyEntry(entry)} title="Copy">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1"/></svg>
            </button>
            <button class="btn-icon" on:click={() => openEdit(entry)} title="Edit">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M11 4H4a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 013 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
            </button>
            <button class="btn-icon btn-danger" on:click={() => remove(entry.Service, entry.Name)} title="Delete">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6m3 0V4a2 2 0 012-2h4a2 2 0 012 2v2"/></svg>
            </button>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>

{#if showAdd}
  <div class="modal-backdrop" on:click|self={() => { showAdd = false; addError = '' }}>
    <div class="modal" on:click|stopPropagation>
      <div class="modal-header">{editing ? 'Edit API Key' : 'Add API Key'}</div>
      <div class="modal-body">
        {#if addError}<div class="modal-error">{addError}</div>{/if}
        <input type="text" placeholder="Service (e.g. OpenAI)" bind:value={addService} />
        <input type="text" placeholder="Key name (e.g. Production)" bind:value={addName} />
        <div style="display:flex; gap:6px; align-items:center;">
          <input type="password" placeholder="API Key" bind:value={addKey} style="flex:1;"
            on:keydown={(e) => e.key === 'Enter' && save()} />
          <button class="btn btn-sm" on:click={generateKey} title="Generate key" style="white-space:nowrap;">Generate</button>
        </div>
        <input type="text" placeholder="Notes (optional)" bind:value={addNotes}
          on:keydown={(e) => e.key === 'Enter' && save()} />
      </div>
      <div class="modal-footer">
        <button class="btn" on:click={() => { showAdd = false; addError = '' }}>Cancel</button>
        <button class="btn btn-primary" on:click={save}>{editing ? 'Save' : 'Add'}</button>
      </div>
    </div>
  </div>
{/if}
