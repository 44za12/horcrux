<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import logoUrl from './logo.png'
  import LockScreen from './components/LockScreen.svelte'
  import VaultList from './components/VaultList.svelte'
  import TotpList from './components/TotpList.svelte'
  import ApiKeyList from './components/ApiKeyList.svelte'
  import FileList from './components/FileList.svelte'
  import Import from './components/Import.svelte'
  import Providers from './components/Providers.svelte'
  import DistributeRestore from './components/DistributeRestore.svelte'
  import { Lock, IsCLIInstalled, InstallCLI, GetAutoLockTimeout } from '../wailsjs/go/main/App'

  let locked = true
  let view = 'vault'
  let error = ''
  let cliInstalled = true
  let installing = false
  let installError = ''
  let installSuccess = false

  let vaultList: any = null
  let totpList: any = null
  let apiKeyList: any = null
  let fileList: any = null

  let idleTimer: any = null
  let idleGraceTimer: any = null
  let idleWarning = false
  let idleGraceSecs = 0
  let autoLockMinutes = 0

  const sections = [
    { id: 'vault', label: 'Passwords', icon: '&#x1F511;', shortcut: '1' },
    { id: 'totp', label: 'Authenticator', icon: '&#x23F1;', shortcut: '2' },
    { id: 'apikeys', label: 'API Keys', icon: '&#x1F510;', shortcut: '3' },
    { id: 'files', label: 'Files', icon: '&#x1F4C1;', shortcut: '4' },
    { id: 'import', label: 'Import', icon: '&#x1F4E5;', shortcut: '5' },
    { id: 'providers', label: 'Providers', icon: '&#x2601;', shortcut: '6' },
    { id: 'distribute', label: 'Distribute', icon: '&#x1F4E1;', shortcut: '7' },
  ]

  async function checkCLI() {
    try { cliInstalled = await IsCLIInstalled() } catch { cliInstalled = false }
  }

  async function handleUnlock(e: CustomEvent) {
    locked = false
    error = ''
    await checkCLI()
    startIdleTimer()
  }

  async function handleLock() {
    clearIdleTimer()
    idleWarning = false
    await Lock()
    locked = true
    view = 'vault'
  }

  function clearIdleTimer() {
    if (idleTimer) { clearTimeout(idleTimer); idleTimer = null }
    if (idleGraceTimer) { clearInterval(idleGraceTimer); idleGraceTimer = null }
  }

  function resetIdleTimer() {
    if (locked || autoLockMinutes === 0) return
    idleWarning = false
    clearIdleTimer()
    const ms = autoLockMinutes * 60 * 1000
    const graceMs = 30 * 1000 // show warning 30s before lock
    idleTimer = setTimeout(() => {
      idleWarning = true
      idleGraceSecs = 30
      idleGraceTimer = setInterval(() => {
        idleGraceSecs--
        if (idleGraceSecs <= 0) {
          clearIdleTimer()
          handleLock()
        }
      }, 1000)
    }, ms - graceMs)
  }

  async function startIdleTimer() {
    try {
      autoLockMinutes = await GetAutoLockTimeout()
    } catch { autoLockMinutes = 5 }
    resetIdleTimer()
  }

  function handleActivity() {
    if (!locked) resetIdleTimer()
  }

  async function handleInstallCLI() {
    installing = true
    installError = ''
    installSuccess = false
    try {
      await InstallCLI()
      installSuccess = true
      await checkCLI()
    } catch (e: any) {
      installError = e.toString().replace('Error: ', '')
    }
    installing = false
  }

  function handleKeydown(e: KeyboardEvent) {
    if (locked) return
    const meta = e.metaKey || e.ctrlKey

    if (meta && e.key === 'l') {
      e.preventDefault()
      handleLock()
      return
    }

    if (meta && e.key === 'n') {
      e.preventDefault()
      if (view === 'vault' && vaultList) vaultList.triggerAdd()
      else if (view === 'totp' && totpList) totpList.triggerAdd()
      else if (view === 'apikeys' && apiKeyList) apiKeyList.triggerAdd()
      else if (view === 'files' && fileList) fileList.triggerAdd()
      return
    }

    if (meta && e.key === 'f') {
      e.preventDefault()
      if (view === 'vault' && vaultList) vaultList.focusSearch()
      else if (view === 'totp' && totpList) totpList.focusSearch()
      else if (view === 'apikeys' && apiKeyList) apiKeyList.focusSearch()
      else if (view === 'files' && fileList) fileList.focusSearch()
      return
    }

    if (meta && e.key >= '1' && e.key <= '7') {
      e.preventDefault()
      const idx = parseInt(e.key) - 1
      if (sections[idx]) view = sections[idx].id
      return
    }
  }

  onMount(() => {
    window.addEventListener('keydown', handleKeydown)
    window.addEventListener('mousemove', handleActivity)
    window.addEventListener('mousedown', handleActivity)
    window.addEventListener('scroll', handleActivity)
    window.addEventListener('click', handleActivity)
    window.addEventListener('touchstart', handleActivity)
  })
  onDestroy(() => {
    window.removeEventListener('keydown', handleKeydown)
    window.removeEventListener('mousemove', handleActivity)
    window.removeEventListener('mousedown', handleActivity)
    window.removeEventListener('scroll', handleActivity)
    window.removeEventListener('click', handleActivity)
    window.removeEventListener('touchstart', handleActivity)
    clearIdleTimer()
  })
</script>

{#if locked}
  <LockScreen on:unlock={handleUnlock} {error} />
{:else}
  <div class="app-shell">
    <nav class="sidebar">
      <div class="sidebar-brand">
        <span class="brand-icon">
          <img src={logoUrl} width="22" height="22" alt="Horcrux" />
        </span>
        <span class="brand-text">Horcrux</span>
      </div>

      <div class="sidebar-section">
        <div class="sidebar-label">Vault</div>
        {#each sections.slice(0, 4) as item}
          <button
            class="nav-btn"
            class:active={view === item.id}
            on:click={() => view = item.id}
          >
            <span class="icon">{@html item.icon}</span>
            <span style="flex:1;">{item.label}</span>
            <span class="shortcut">⌘{item.shortcut}</span>
          </button>
        {/each}
      </div>

      <div class="sidebar-section">
        <div class="sidebar-label">Tools</div>
        {#each sections.slice(4, 5) as item}
          <button
            class="nav-btn"
            class:active={view === item.id}
            on:click={() => view = item.id}
          >
            <span class="icon">{@html item.icon}</span>
            <span style="flex:1;">{item.label}</span>
            <span class="shortcut">⌘{item.shortcut}</span>
          </button>
        {/each}
      </div>

      <div class="sidebar-section">
        <div class="sidebar-label">Sync</div>
        {#each sections.slice(5) as item}
          <button
            class="nav-btn"
            class:active={view === item.id}
            on:click={() => view = item.id}
          >
            <span class="icon">{@html item.icon}</span>
            <span style="flex:1;">{item.label}</span>
            <span class="shortcut">⌘{item.shortcut}</span>
          </button>
        {/each}
      </div>

      <div class="sidebar-footer">
        {#if !cliInstalled}
          <button class="nav-btn" on:click={handleInstallCLI} disabled={installing}>
            <span class="icon">&#x1F4E5;</span>
            {installing ? 'Installing...' : 'Install CLI'}
          </button>
          {#if installError}
            <div class="cli-error">{installError}</div>
          {/if}
          {#if installSuccess}
            <div class="cli-success">CLI installed to /usr/local/bin</div>
          {/if}
        {/if}
        {#if idleWarning}
          <div class="idle-warning">
            <span class="icon">&#x23F0;</span>
            Auto-lock in {idleGraceSecs}s
          </div>
        {/if}
        <button class="nav-btn" on:click={handleLock}>
          <span class="icon">&#x1F512;</span>
          <span style="flex:1;">Lock Vault</span>
          <span class="shortcut">⌘L</span>
        </button>
      </div>
    </nav>

    <div class="main">
      <div class="content fade-in" key={view}>
        {#if view === 'vault'}<VaultList bind:this={vaultList} />
        {:else if view === 'totp'}<TotpList bind:this={totpList} />
        {:else if view === 'apikeys'}<ApiKeyList bind:this={apiKeyList} />
        {:else if view === 'files'}<FileList bind:this={fileList} />
        {:else if view === 'import'}<Import />
        {:else if view === 'providers'}<Providers />
        {:else if view === 'distribute'}<DistributeRestore />
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  .shortcut {
    font-size: 10px;
    color: var(--text-tertiary);
    background: var(--bg-inline);
    padding: 1px 5px;
    border-radius: 3px;
    font-family: var(--font-mono);
    opacity: 0.7;
  }
  .nav-btn:hover .shortcut { opacity: 1; }
  .cli-error {
    font-size: 10px;
    color: var(--red);
    padding: 2px 8px;
    margin-top: -2px;
  }
  .cli-success {
    font-size: 10px;
    color: var(--green);
    padding: 2px 8px;
    margin-top: -2px;
  }
  .idle-warning {
    font-size: 11px;
    color: var(--orange);
    padding: 4px 8px;
    display: flex;
    align-items: center;
    gap: 6px;
    animation: pulse 1s ease-in-out infinite;
  }
  .idle-warning .icon { font-size: 14px; }
  @keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.6; }
  }
</style>
