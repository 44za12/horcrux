<script lang="ts">
  import { onMount } from 'svelte'
  import { GetDistributeStatus, Distribute, Restore } from '../../wailsjs/go/main/App'

  let status: any = null
  let loading = true
  let busy = false
  let msg = ''
  let msgType = ''
  let showRestoreConfirm = false

  async function load() {
    try {
      status = await GetDistributeStatus()
    } catch (e: any) {
      msg = e.toString().replace('Error: ', ''); msgType = 'error'
    }
    loading = false
  }

  async function distributeVault() {
    busy = true; msg = ''
    try {
      await Distribute()
      msg = 'Vault distributed successfully across all providers'; msgType = 'success'
      await load()
    } catch (e: any) { msg = e.toString().replace('Error: ', ''); msgType = 'error' }
    busy = false
  }

  async function restoreVault() {
    busy = true; msg = ''
    try {
      await Restore()
      msg = 'Vault restored successfully'; msgType = 'success'
    } catch (e: any) { msg = e.toString().replace('Error: ', ''); msgType = 'error' }
    busy = false
  }

  onMount(load)
</script>

<div class="slide-up distribute-center">
  {#if loading}
    <div class="empty-icon">&#x23F3;</div>
    <div>Loading...</div>
  {:else if !status || status.Total < 3}
    <div class="distribute-icon">&#x1F4E1;</div>
    <div class="distribute-title">Distribute &amp; Restore</div>
    <div class="msg-error" style="margin-top:8px;">
      Need at least 3 providers to distribute. Currently have {status ? status.Total : 0}.
    </div>
    <div class="empty-desc" style="margin-top:4px;">
      Go to the Providers tab to add storage locations.
    </div>
  {:else}
    <div class="distribute-icon">&#x1F4E1;</div>
    <div class="distribute-title">Distribute &amp; Restore</div>

    <div class="distribute-stat">
      <div class="distribute-stat-item">
        <div class="distribute-stat-value">{status.Total}</div>
        <div class="distribute-stat-label">Providers</div>
      </div>
      <div class="distribute-stat-item">
        <div class="distribute-stat-value">{status.Threshold}</div>
        <div class="distribute-stat-label">Threshold</div>
      </div>
      <div class="distribute-stat-item">
        <div class="distribute-stat-value">{status.Failures}</div>
        <div class="distribute-stat-label">Tolerated Failures</div>
      </div>
    </div>

    {#if msg}
      <div class="msg-{msgType}" style="width:100%; max-width:400px; text-align:center; margin-bottom:8px;">
        {msg}
      </div>
    {/if}

    <div class="distribute-actions">
      <button class="btn btn-primary" on:click={distributeVault} disabled={!status.CanDistribute || busy}>
        {busy ? 'Working...' : 'Distribute Vault'}
      </button>
      {#if showRestoreConfirm}
        <div class="restore-confirm-group">
          <span class="restore-warning">This will overwrite your local vault. Continue?</span>
          <button class="btn btn-danger" on:click={() => { showRestoreConfirm = false; restoreVault() }} disabled={busy}>
            {busy ? 'Restoring...' : 'Yes, Restore'}
          </button>
          <button class="btn" on:click={() => showRestoreConfirm = false} disabled={busy}>
            Cancel
          </button>
        </div>
      {:else}
        <button class="btn" on:click={() => showRestoreConfirm = true} disabled={!status.CanRestore || busy}>
          Restore Vault
        </button>
      {/if}
    </div>
  {/if}
</div>

<style>
  .restore-confirm-group {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
  }
  .restore-warning {
    font-size: 13px;
    color: var(--red);
    font-weight: 500;
    text-align: center;
  }
  .btn-danger {
    background: var(--red);
    color: #fff;
    border: none;
  }
  .btn-danger:hover {
    opacity: 0.85;
  }
</style>
