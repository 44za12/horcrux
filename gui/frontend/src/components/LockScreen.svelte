<script lang="ts">
  import { onMount, createEventDispatcher } from 'svelte'
  import logoUrl from '../logo.png'
  import {
    UnlockWithBiometric,
    HasBiometricKey,
    IsInitialized,
    CreateVaultWithPassphrase,
    UnlockWithPassphrase
  } from '../../wailsjs/go/main/App'

  export let error = ''
  let passphrase = ''
  let hasStoredTouchIdKey = false
  let isInit = true
  let showPassphraseInput = false
  let showCreateInput = false
  let newPassphrase = ''
  let confirmPassphrase = ''
  let creating = false
  let splashDone = false

  const dispatch = createEventDispatcher()

  async function checkState() {
    try {
      isInit = await IsInitialized()
      if (isInit) {
        hasStoredTouchIdKey = await HasBiometricKey()
      }
    } catch {}
    setTimeout(() => { splashDone = true }, 800)
    if (isInit && hasStoredTouchIdKey) {
      await biometricUnlock()
    }
  }

  async function biometricUnlock() {
    try {
      await UnlockWithBiometric()
      dispatch('unlock', '')
    } catch (e: any) {
      error = e.toString().replace('Error: ', '')
      showPassphraseInput = true
    }
  }

  async function createNewVault() {
    if (!newPassphrase) return
    if (newPassphrase !== confirmPassphrase) {
      error = 'Passphrases do not match'
      return
    }
    creating = true
    error = ''
    try {
      await CreateVaultWithPassphrase(newPassphrase)
      dispatch('unlock', '')
    } catch (e: any) {
      error = e.toString().replace('Error: ', '')
    }
    creating = false
  }

  async function submitPassphrase() {
    if (!passphrase) return
    try {
      await UnlockWithPassphrase(passphrase)
      dispatch('unlock', '')
    } catch (e: any) {
      error = e.toString().replace('Error: ', '')
    }
  }

  onMount(checkState)
</script>

{#if !splashDone}
  <div class="splash">
    <div class="splash-icon">
      <img src={logoUrl} width="48" height="48" alt="Horcrux" />
    </div>
  </div>
{:else}
  <div class="lock-screen fade-in">
    <div class="lock-logo">
      <img src={logoUrl} width="36" height="36" alt="Horcrux" />
    </div>
    <div class="lock-title">Horcrux</div>

    {#if !isInit}
      <div class="lock-subtitle">No vault found.</div>
      {#if error}
        <div class="lock-error">{error}</div>
      {:else}
        <div class="lock-error">&nbsp;</div>
      {/if}
      {#if showCreateInput}
        <div class="lock-input">
          <input
            type="password"
            placeholder="New passphrase"
            bind:value={newPassphrase}
            on:keydown={(e) => e.key === 'Enter' && createNewVault()}
          />
        </div>
        <div class="lock-input" style="margin-top:8px;">
          <input
            type="password"
            placeholder="Confirm passphrase"
            bind:value={confirmPassphrase}
            on:keydown={(e) => e.key === 'Enter' && createNewVault()}
          />
        </div>
        <button class="lock-btn" on:click={createNewVault} disabled={creating || !newPassphrase || !confirmPassphrase}>
          {creating ? 'Creating...' : 'Create Vault'}
        </button>
        <button class="lock-btn lock-btn-secondary" on:click={() => { showCreateInput = false; error = ''; newPassphrase = ''; confirmPassphrase = '' }}>
          Back
        </button>
      {:else}
        <button class="lock-btn" on:click={() => { showCreateInput = true; error = '' }}>
          Get Started
        </button>
      {/if}
      <div class="lock-hint">This is the only passphrase you need for unlock, distribute, and restore. Touch ID is enabled after first unlock.</div>

    {:else if !showPassphraseInput}
      <div class="lock-subtitle">Unlock your vault</div>
      {#if error}
        <div class="lock-error">{error}</div>
      {:else}
        <div class="lock-error">&nbsp;</div>
      {/if}

      {#if hasStoredTouchIdKey}
        <button class="lock-btn" on:click={biometricUnlock}>
          Touch ID
        </button>
        <button class="lock-btn lock-btn-secondary" on:click={() => { showPassphraseInput = true; error = '' }}>
          Use passphrase
        </button>
      {:else}
        <div class="lock-input">
          <input
            type="password"
            placeholder="Passphrase"
            bind:value={passphrase}
            on:keydown={(e) => e.key === 'Enter' && submitPassphrase()}
          />
        </div>
        <button class="lock-btn" on:click={submitPassphrase} disabled={!passphrase}>Unlock</button>
      {/if}

    {:else}
      <div class="lock-subtitle">Enter your passphrase</div>
      {#if error}
        <div class="lock-error">{error}</div>
      {:else}
        <div class="lock-error">&nbsp;</div>
      {/if}
      <div class="lock-input">
        <input
          type="password"
          placeholder="Passphrase"
          bind:value={passphrase}
          on:keydown={(e) => e.key === 'Enter' && submitPassphrase()}
        />
      </div>
      <button class="lock-btn" on:click={submitPassphrase} disabled={!passphrase}>Unlock</button>
      <button class="lock-btn lock-btn-secondary" on:click={() => { showPassphraseInput = false; error = ''; passphrase = '' }}>
        Back
      </button>
    {/if}
  </div>
{/if}

<style>
  .splash {
    height: 100%;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    background: var(--bg);
    animation: splashFade 1s ease-out;
  }
  .splash-icon {
    color: var(--accent);
    animation: splashScale 0.6s cubic-bezier(0.16, 1, 0.3, 1) 0.1s both;
  }
  @keyframes splashScale {
    from { opacity: 0; transform: scale(0.7); }
    to { opacity: 1; transform: scale(1); }
  }
  @keyframes splashFade {
    from { opacity: 0; transform: scale(0.95); }
    to { opacity: 1; transform: scale(1); }
  }
  .lock-hint {
    font-size: 11px;
    color: var(--text-tertiary);
    max-width: 260px;
    text-align: center;
    margin-top: 6px;
  }
  .lock-btn-secondary {
    margin-top: 6px;
    background: var(--bg-elevated);
    color: var(--text-secondary);
    border: 1px solid var(--border);
    font-size: 12px;
  }
  .lock-btn-secondary:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
</style>
