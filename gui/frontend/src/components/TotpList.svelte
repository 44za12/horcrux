<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { ListTotpServices, GetTotpCode, GetTotpSecondsRemaining, AddTotp, RemoveTotp } from '../../wailsjs/go/main/App'

  let services: any[] = []
  let codes: Record<string, string> = {}
  let secondsLeft = 30
  let loading = true
  let timer: any = null
  let search = ''
  let showAdd = false
  let addName = ''
  let addSecret = ''
  let addError = ''
  let confirmRemove = ''
  let removing = ''

  let searchEl: any = null

  export function triggerAdd() { showAdd = true }
  export function focusSearch() { searchEl && searchEl.focus() }

  $: filtered = search
    ? services.filter((s: any) => s.Name.toLowerCase().includes(search.toLowerCase()))
    : services

  async function load() {
    try {
      services = await ListTotpServices() || []
      secondsLeft = await GetTotpSecondsRemaining()
      await refresh()
    } catch (e) { console.error(e) }
    loading = false
  }

  async function refresh() {
    try {
      for (const s of filtered) {
        codes[s.Name] = await GetTotpCode(s.Name)
      }
      codes = { ...codes }
    } catch (e) { console.error(e) }
  }

  $: pct = (secondsLeft / 30) * 100
  $: ringClass = secondsLeft <= 5 ? 'critical' : secondsLeft <= 10 ? 'warning' : ''

  async function copy(text: string) {
    try { await navigator.clipboard.writeText(text) }
    catch {
      const t = document.createElement('textarea'); t.value = text
      document.body.appendChild(t); t.select(); document.execCommand('copy'); document.body.removeChild(t)
    }
  }

  async function add() {
    if (!addName || !addSecret) { addError = 'Service name and secret are required'; return }
    try {
      await AddTotp(addName, addSecret)
      showAdd = false; addName = ''; addSecret = ''; addError = ''
      await load()
    } catch (e: any) { addError = e.toString().replace('Error: ', '') }
  }

  async function remove(name: string) {
    removing = name
    try { await RemoveTotp(name); await load() } catch (e) { console.error(e) }
    removing = ''; confirmRemove = ''
  }

  onMount(() => {
    load()
    timer = setInterval(async () => {
      try {
        secondsLeft = await GetTotpSecondsRemaining()
        if (secondsLeft === 30) await refresh()
      } catch (e) { console.error(e) }
    }, 1000)
  })

  onDestroy(() => { if (timer) clearInterval(timer) })
</script>

<div class="slide-up">
  <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:14px;">
    <div style="display:flex; align-items:center; gap:10px;">
      <div class="search-wrap">
        <input class="search-input" type="search" placeholder="Search authenticators..." bind:value={search} bind:this={searchEl} />
      </div>
      {#if search}
        <span style="font-size:12px; color:var(--text-tertiary);">{filtered.length} results</span>
      {:else}
        <span style="font-size:12px; color:var(--text-tertiary);">{services.length} service{services.length !== 1 ? 's' : ''}</span>
      {/if}
    </div>
    <button class="btn btn-primary btn-sm" on:click={() => showAdd = true}>+ Add TOTP</button>
  </div>

  {#if loading}
    <div class="empty">
      <div class="empty-icon">&#x23F3;</div>
      <div class="empty-title">Loading...</div>
    </div>
  {:else if filtered.length === 0}
    <div class="empty">
      <div class="empty-icon">&#x23F1;</div>
      <div class="empty-title">{search ? 'No matches' : 'No authenticator codes'}</div>
      <div class="empty-desc">{search ? 'Try a different search term' : 'Click "+ Add TOTP" to add a 2FA service.'}</div>
    </div>
  {:else}
    <div class="totp-grid">
      {#each filtered as svc}
        <div class="totp-card">
          <div class="totp-name" title={svc.Name}>{svc.Name}</div>
          <div class="totp-code">{codes[svc.Name] || '------'}</div>
          <div class="totp-ring">
            <div class="totp-ring-fill {ringClass}" style="width:{pct}%"></div>
          </div>
          <div style="display:flex; gap:4px;">
            <button class="btn btn-sm" style="flex:1;" on:click={() => copy(codes[svc.Name] || '')}>
              Copy
            </button>
            {#if confirmRemove === svc.Name}
              <button class="btn btn-sm btn-danger" on:click={() => remove(svc.Name)} disabled={removing === svc.Name}>
                {removing === svc.Name ? '...' : 'Yes'}
              </button>
              <button class="btn btn-sm" on:click={() => confirmRemove = ''}>No</button>
            {:else}
              <button class="btn btn-sm btn-danger" on:click={() => confirmRemove = svc.Name}>Remove</button>
            {/if}
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>

{#if showAdd}
  <div class="modal-backdrop" on:click|self={() => { showAdd = false; addError = '' }}>
    <div class="modal" on:click|stopPropagation>
      <div class="modal-header">Add TOTP Authenticator</div>
      <div class="modal-body">
        {#if addError}<div class="modal-error">{addError}</div>{/if}
        <input type="text" placeholder="Service name (e.g. GitHub)" bind:value={addName} />
        <input type="text" placeholder="Secret key (Base32)" bind:value={addSecret}
          on:keydown={(e) => e.key === 'Enter' && add()} />
        <div style="font-size:11px; color:var(--text-tertiary); margin-top:-2px;">
          Get the secret key from the service's 2FA setup page.
        </div>
      </div>
      <div class="modal-footer">
        <button class="btn" on:click={() => { showAdd = false; addError = '' }}>Cancel</button>
        <button class="btn btn-primary" on:click={add}>Add</button>
      </div>
    </div>
  </div>
{/if}
