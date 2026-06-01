<script lang="ts">
  import { onMount } from 'svelte'
  import {
    ListProviders,
    RemoveProvider,
    GetProviderTypes,
    AddProvider
  } from '../../wailsjs/go/main/App'

  let providers: any[] = []
  let loading = true
  let removing = ''
  let confirmRemove = ''
  let showAdd = false
  let adding = false
  let addError = ''
  let error = ''

  let selectedType = ''
  let providerTypes: any[] = []

  let form: Record<string, string> = {}

  const fieldsByType: Record<string, { key: string; label: string; placeholder: string; type?: string }[]> = {
    local: [
      { key: 'Path', label: 'Path', placeholder: 'Default: ~/.horcrux/distributed' },
    ],
    s3: [
      { key: 'Endpoint', label: 'Endpoint', placeholder: 's3.amazonaws.com' },
      { key: 'Region', label: 'Region', placeholder: 'us-east-1' },
      { key: 'Bucket', label: 'Bucket', placeholder: 'my-bucket' },
      { key: 'AccessKey', label: 'Access Key ID', placeholder: '' },
      { key: 'SecretKey', label: 'Secret Access Key', placeholder: '', type: 'password' },
    ],
    usb: [
      { key: 'Path', label: 'Mount Path', placeholder: '/Volumes/USBDRIVE' },
    ],
    ssh: [
      { key: 'Host', label: 'Host', placeholder: 'example.com' },
      { key: 'Port', label: 'Port', placeholder: '22' },
      { key: 'Username', label: 'Username', placeholder: '' },
      { key: 'Password', label: 'Password', placeholder: '', type: 'password' },
      { key: 'KeyPath', label: 'SSH Key Path', placeholder: '~/.ssh/id_rsa (optional)' },
      { key: 'RemotePath', label: 'Remote Path', placeholder: '.horcrux' },
    ],
    webdav: [
      { key: 'Endpoint', label: 'URL', placeholder: 'https://nextcloud.example.com/remote.php/dav/files/user' },
      { key: 'Username', label: 'Username', placeholder: '' },
      { key: 'Password', label: 'Password / App Token', placeholder: '', type: 'password' },
    ],
  }

  const icons: Record<string, string> = {
    local: '&#x1F4C1;', gdrive: '&#x1F4C2;', dropbox: '&#x1F4E6;',
    s3: '&#x2601;', usb: '&#x1F4BE;', ssh: '&#x1F5A5;', webdav: '&#x1F310;',
  }

  function statusBadge(s: string) {
    if (s === 'ready' || s === 'authenticated') return 'badge-green'
    if (s.includes('expired')) return 'badge-orange'
    return 'badge-blue'
  }

  $: needsName = selectedType !== 'local'
  $: currentFields = fieldsByType[selectedType] || []
  $: selectedTypeName = providerTypes.find((t: any) => t.Id === selectedType)?.Name || selectedType

  async function load() {
    error = ''
    try {
      providers = await ListProviders() || []
    } catch (e: any) {
      error = e.toString().replace('Error: ', '')
    }
    loading = false
  }

  function openAdd() {
    selectedType = ''
    form = {}
    addError = ''
    showAdd = true
    adding = false
    GetProviderTypes().then(t => providerTypes = t || []).catch(() => {})
  }

  function selectType(t: string) {
    selectedType = t
    form = {}
    addError = ''
  }

  async function doAdd() {
    adding = true
    addError = ''
    try {
      const req: Record<string, string> = {
        ProviderType: selectedType,
      }
      if (form.Name) req.Name = form.Name
      for (const f of currentFields) {
        if (form[f.key]) req[f.key] = form[f.key]
      }
      await AddProvider(req)
      showAdd = false
      await load()
    } catch (e: any) {
      addError = e.toString().replace('Error: ', '')
    }
    adding = false
  }

  async function remove(name: string) {
    removing = name
    try { await RemoveProvider(name); await load() } catch (e: any) { error = e.toString().replace('Error: ', '') }
    removing = ''; confirmRemove = ''
  }

  onMount(load)
</script>

<div class="slide-up">
  {#if error}
    <div class="msg-error" style="margin-bottom:10px;">{error}</div>
  {/if}

  <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:14px;">
    <span style="font-size:12px; color:var(--text-tertiary);">
      {providers.length} provider{providers.length !== 1 ? 's' : ''} configured
    </span>
    <button class="btn btn-primary btn-sm" on:click={openAdd}>+ Add Provider</button>
  </div>

  {#if loading}
    <div class="empty">
      <div class="empty-icon">&#x23F3;</div>
      <div class="empty-title">Loading...</div>
    </div>
  {:else if providers.length === 0}
    <div class="empty">
      <div class="empty-icon">&#x2601;</div>
      <div class="empty-title">No providers</div>
      <div class="empty-desc">Click "Add Provider" to configure your first storage location.</div>
    </div>
  {:else}
    <div class="provider-grid">
      {#each providers as p}
        <div class="card">
          <div class="card-row">
            <div class="provider-icon">{@html icons[p.Type] || '&#x2601;'}</div>
            <div class="provider-info">
              <div class="provider-name">{p.Name}</div>
              <div class="provider-type">{p.Type}</div>
            </div>
            <span class="badge {statusBadge(p.Status)}">{p.Status}</span>
          </div>
          <div style="display:flex; justify-content:flex-end; margin-top:10px; border-top:1px solid var(--divider); padding-top:8px;">
            {#if confirmRemove === p.Name}
              <span style="font-size:11px; color:var(--red); margin-right:auto; align-self:center;">Remove?</span>
              <button class="btn btn-sm btn-danger" on:click={() => remove(p.Name)} disabled={removing === p.Name}>
                {removing === p.Name ? '...' : 'Yes'}
              </button>
              <button class="btn btn-sm" on:click={() => confirmRemove = ''}>No</button>
            {:else}
              <button class="btn btn-sm btn-danger" on:click={() => confirmRemove = p.Name}>Remove</button>
            {/if}
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>

{#if showAdd}
  <div class="modal-backdrop" on:click|self={() => showAdd = false}>
    <div class="modal" style="width:460px;" on:click|stopPropagation>
      <div class="modal-header">Add Provider</div>
      <div class="modal-body">
        {#if addError}
          <div class="modal-error">{addError}</div>
        {/if}

        {#if !selectedType}
          <div style="margin-bottom:8px; font-size:12px; color:var(--text-secondary);">Choose a provider type:</div>
          <div class="add-type-grid">
            {#each providerTypes as pt}
              <button class="add-type-card" on:click={() => selectType(pt.Id)}>
                <span class="add-type-icon">{@html icons[pt.Id] || '&#x2601;'}</span>
                <span class="add-type-name">{pt.Name}</span>
              </button>
            {/each}
          </div>
        {:else if selectedType === 'gdrive' || selectedType === 'dropbox'}
          <div style="text-align:center; padding:16px 0;">
            <div style="font-size:32px; margin-bottom:8px;">{@html icons[selectedType]}</div>
            <div style="font-size:14px; font-weight:600; margin-bottom:4px;">
              {selectedType === 'gdrive' ? 'Google Drive' : 'Dropbox'}
            </div>
            <div style="font-size:12px; color:var(--text-secondary); margin-bottom:12px;">
              A browser window will open for OAuth authentication.
            </div>
            {#if needsName}
              <input
                type="text"
                placeholder="Provider name (optional)"
                bind:value={form.Name}
              />
            {/if}
          </div>
        {:else}
          <div style="margin-bottom:10px;">
            <button class="btn btn-sm" on:click={() => { selectedType = ''; form = {}; addError = ''; }}>&larr; Back</button>
              <span style="margin-left:8px; font-weight:600;">{@html icons[selectedType]} {selectedTypeName}</span>
          </div>
          {#if needsName}
            <input
              type="text"
              placeholder="Provider name (leave empty for default)"
              bind:value={form.Name}
            />
          {/if}
          {#each currentFields as f}
            <input
              type="{f.type || 'text'}"
              placeholder="{f.placeholder || f.label}"
              value={form[f.key] || ''}
              on:input={(e) => { form[f.key] = e.target.value; form = form }}
            />
          {/each}
        {/if}
      </div>
      <div class="modal-footer">
        <button class="btn" on:click={() => showAdd = false}>Cancel</button>
        {#if selectedType}
          <button class="btn btn-primary" on:click={doAdd} disabled={adding}>
            {adding ? 'Connecting...' : selectedType === 'gdrive' || selectedType === 'dropbox' ? 'Authenticate' : 'Add Provider'}
          </button>
        {/if}
      </div>
    </div>
  </div>
{/if}
