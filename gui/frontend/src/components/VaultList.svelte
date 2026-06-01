<script lang="ts">
  import { onMount } from 'svelte'
  import { ListPasswords, GetPassword, AddPassword, RemovePassword, GeneratePassword } from '../../wailsjs/go/main/App'

  let passwords: any[] = []
  let search = ''
  let loading = true
  let revealed: Record<string, boolean> = {}
  let showAdd = false
  let addSite = ''
  let addUser = ''
  let addPass = ''
  let addNotes = ''
  let addError = ''
  let editing = false
  let editOrigSite = ''
  let editOrigUser = ''

  $: filtered = search
    ? passwords.filter((p: any) =>
        p.Site.toLowerCase().includes(search.toLowerCase()) ||
        p.Username.toLowerCase().includes(search.toLowerCase()) ||
        (p.Notes && p.Notes.toLowerCase().includes(search.toLowerCase()))
      )
    : passwords

  let searchEl: any = null

  export function triggerAdd() { openAdd() }
  export function focusSearch() { searchEl && searchEl.focus() }

  async function load() {
    try { passwords = await ListPasswords() || [] } catch (e: any) { console.error(e) }
    loading = false
  }

  let clipboardTimer: any = null

  async function copy(text: string) {
    try { await navigator.clipboard.writeText(text) }
    catch {
      const t = document.createElement('textarea'); t.value = text
      document.body.appendChild(t); t.select(); document.execCommand('copy'); document.body.removeChild(t)
    }
    // Clear clipboard after 30 seconds to prevent stale secrets
    if (clipboardTimer) clearTimeout(clipboardTimer)
    clipboardTimer = setTimeout(async () => {
      try { await navigator.clipboard.writeText('') } catch {}
    }, 30000)
  }

  async function copyEntry(entry: any) {
    const password = await GetPassword(entry.Site, entry.Username)
    await copy(password)
  }

  async function reveal(entry: any) {
    const key = entry.Site + entry.Username
    entry.Password = await GetPassword(entry.Site, entry.Username)
    revealed[key] = true
    passwords = [...passwords]
    setTimeout(() => {
      entry.Password = ''
      revealed = { ...revealed, [key]: false }
      passwords = [...passwords]
    }, 5000)
  }

  async function generatePass() {
    try { addPass = await GeneratePassword() } catch (e) { console.error(e) }
  }

  function openAdd() {
    editing = false
    addSite = ''; addUser = ''; addPass = ''; addNotes = ''; addError = ''
    showAdd = true
  }

  async function openEdit(entry: any) {
    editing = true
    editOrigSite = entry.Site; editOrigUser = entry.Username
    addSite = entry.Site; addUser = entry.Username; addPass = await GetPassword(entry.Site, entry.Username); addNotes = entry.Notes || ''; addError = ''
    showAdd = true
  }

  async function save() {
    if (!addSite || !addUser || !addPass) { addError = 'Site, username, and password are required'; return }
    try {
      if (editing) {
        if (editOrigSite !== addSite || editOrigUser !== addUser) {
          await RemovePassword(editOrigSite, editOrigUser)
        }
      }
      await AddPassword(addSite, addUser, addPass, addNotes)
      showAdd = false; addSite = ''; addUser = ''; addPass = ''; addNotes = ''; addError = ''
      editing = false
      await load()
    } catch (e: any) { addError = e.toString().replace('Error: ', '') }
  }

  async function remove(site: string, username: string) {
    try { await RemovePassword(site, username); await load() } catch (e) { console.error(e) }
  }

  onMount(load)
</script>

<div class="slide-up">
  <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:14px;">
    <div style="display:flex; align-items:center; gap:10px;">
      <div class="search-wrap">
        <input class="search-input" type="search" placeholder="Search passwords..." bind:value={search} bind:this={searchEl} />
      </div>
      {#if search}
        <span style="font-size:12px; color:var(--text-tertiary);">{filtered.length} results</span>
      {:else}
        <span style="font-size:12px; color:var(--text-tertiary);">{passwords.length} passwords</span>
      {/if}
    </div>
    <div style="display:flex; gap:6px;">
      <button class="btn btn-primary btn-sm" on:click={openAdd}>+ Add Password</button>
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
      <div class="empty-title">{search ? 'No matches' : 'No passwords yet'}</div>
      <div class="empty-desc">{search ? 'Try a different search term' : 'Click "+ Add Password" or "Import" to get started'}</div>
    </div>
  {:else}
    <div class="card" style="padding:0; overflow:hidden;">
      <div class="list-header">
        <span>Site</span>
        <span>Username</span>
        <span>Password</span>
        <span>Notes</span>
        <span></span>
      </div>
      {#each filtered as entry}
        <div class="list-row">
          <div class="list-cell" style="font-weight:600;">{entry.Site}</div>
          <div class="list-cell" style="color:var(--text-secondary);">{entry.Username}</div>
          <div class="list-cell list-cell-mono">
            {revealed[entry.Site + entry.Username] ? entry.Password : '••••••••••'}
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
            <button class="btn-icon btn-danger" on:click={() => remove(entry.Site, entry.Username)} title="Delete">
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
      <div class="modal-header">{editing ? 'Edit Password' : 'Add Password'}</div>
      <div class="modal-body">
        {#if addError}<div class="modal-error">{addError}</div>{/if}
        <input type="text" placeholder="Site (e.g. github.com)" bind:value={addSite} />
        <input type="text" placeholder="Username or email" bind:value={addUser} />
        <div style="display:flex; gap:6px; align-items:center;">
          <input type="password" placeholder="Password" bind:value={addPass} style="flex:1;"
            on:keydown={(e) => e.key === 'Enter' && save()} />
          <button class="btn btn-sm" on:click={generatePass} title="Generate password" style="white-space:nowrap;">Generate</button>
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
