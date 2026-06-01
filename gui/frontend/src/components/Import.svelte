<script lang="ts">
  import { ImportCSV, ImportTOTP } from '../../wailsjs/go/main/App'

  let importMsg = ''
  let importMsgType = ''

  function readFile(file: File): Promise<string> {
    return new Promise((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = () => resolve(reader.result as string)
      reader.onerror = () => reject(reader.error)
      reader.readAsText(file)
    })
  }

  async function handleCSV() {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = '.csv'
    input.onchange = async () => {
      const file = input.files?.[0]
      if (!file) return
      importMsg = 'Importing...'; importMsgType = ''
      try {
        const content = await readFile(file)
        const count = await ImportCSV(content)
        importMsg = `Imported ${count} passwords`; importMsgType = 'success'
      } catch (e: any) {
        importMsg = e.toString().replace('Error: ', ''); importMsgType = 'error'
      }
    }
    input.click()
  }

  async function handleTOTP() {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = '.json,.txt,.text'
    input.onchange = async () => {
      const file = input.files?.[0]
      if (!file) return
      importMsg = 'Importing...'; importMsgType = ''
      try {
        const content = await readFile(file)
        const count = await ImportTOTP(content)
        importMsg = `Imported ${count} services`; importMsgType = 'success'
      } catch (e: any) {
        importMsg = e.toString().replace('Error: ', ''); importMsgType = 'error'
      }
    }
    input.click()
  }
</script>

<div class="slide-up">
  <div style="margin-bottom:20px;">
    <div style="font-size:15px; font-weight:600; margin-bottom:4px;">Import Data</div>
    <div style="font-size:12px; color:var(--text-secondary);">Import passwords and authenticator codes from other apps.</div>
  </div>

  {#if importMsg}
    <div class="msg-{importMsgType}" style="margin-bottom:14px;">{importMsg}</div>
  {/if}

  <div class="import-sections">
    <div class="card import-card">
      <div class="import-card-header">
        <span class="import-icon">&#x1F4C4;</span>
        <div>
          <div class="import-title">Passwords from CSV</div>
          <div class="import-subtitle">Chrome, Firefox, 1Password, Bitwarden, LastPass</div>
        </div>
      </div>
      <div class="import-format">
        <div class="import-format-label">Expected columns:</div>
        <pre class="code-block">url,name,username,password,note</pre>
        <div class="import-format-hint">First row is header. Columns: 0=url, 2=username, 3=password.</div>
      </div>
      <button class="btn btn-primary" on:click={handleCSV}>
        Choose CSV File
      </button>
    </div>

    <div class="card import-card">
      <div class="import-card-header">
        <span class="import-icon">&#x23F1;</span>
        <div>
          <div class="import-title">Authenticator Codes (TOTP)</div>
          <div class="import-subtitle">2FAS, Google Authenticator, Authy, andOTP, Raivo</div>
        </div>
      </div>
      <div class="import-format">
        <div class="import-format-label">Auto-detected formats:</div>
        <div class="format-list">
          <div class="format-item">
            <span class="format-badge">JSON</span>
            <span>2FAS export <code class="code-inline">{`{"services":[{"name":"...","secret":"..."}]}`}</code></span>
          </div>
          <div class="format-item">
            <span class="format-badge">TXT</span>
            <span>One <code class="code-inline">otpauth://totp/...</code> URI per line</span>
          </div>
        </div>
      </div>
      <button class="btn btn-primary" on:click={handleTOTP}>
        Choose File
      </button>
    </div>
  </div>
</div>

<style>
  .import-sections { display: flex; flex-direction: column; gap: 16px; }
  .import-card { padding: 18px; }
  .import-card-header { display: flex; align-items: center; gap: 12px; margin-bottom: 14px; }
  .import-icon {
    font-size: 28px; width: 44px; height: 44px;
    display: flex; align-items: center; justify-content: center;
    background: var(--bg-inline); border-radius: 10px; flex-shrink: 0;
  }
  .import-title { font-size: 14px; font-weight: 600; }
  .import-subtitle { font-size: 11px; color: var(--text-tertiary); margin-top: 1px; }
  .import-format { background: var(--bg-inline); border-radius: var(--radius); padding: 12px; margin-bottom: 12px; }
  .import-format-label {
    font-size: 11px; font-weight: 600; color: var(--text-secondary);
    text-transform: uppercase; letter-spacing: 0.3px; margin-bottom: 8px;
  }
  .import-format-hint { font-size: 11px; color: var(--text-tertiary); margin-top: 6px; }
  .format-list { display: flex; flex-direction: column; gap: 6px; }
  .format-item { display: flex; align-items: center; gap: 8px; font-size: 12px; color: var(--text-secondary); }
  .format-badge {
    font-size: 10px; font-weight: 700; padding: 1px 6px; border-radius: 3px;
    background: var(--accent-light); color: var(--accent); flex-shrink: 0;
  }
  .code-block {
    font-family: var(--font-mono); font-size: 11px; line-height: 1.5;
    color: var(--text-secondary); white-space: pre; overflow-x: auto; margin: 0;
  }
</style>
