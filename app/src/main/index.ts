import { app, BrowserWindow } from 'electron'
import { spawn, ChildProcess } from 'node:child_process'
import { join } from 'node:path'

let backend: ChildProcess | null = null

function startBackend(): void {
  if (process.env.NODE_ENV === 'development' || !app.isPackaged) return
  const exe = join(process.resourcesPath, process.platform === 'win32' ? 'kube-sched-lens.exe' : 'kube-sched-lens')
  const args = process.env.KSL_DEMO === '1' ? ['--demo'] : []
  backend = spawn(exe, args, { stdio: 'ignore' })
  backend.on('error', (err) => console.error('backend failed to start:', err))
}

function createWindow(): void {
  const win = new BrowserWindow({
    width: 1280,
    height: 800,
    backgroundColor: '#111418',
    webPreferences: { contextIsolation: true, nodeIntegration: false }
  })
  win.removeMenu()
  if (process.env['ELECTRON_RENDERER_URL']) {
    win.loadURL(process.env['ELECTRON_RENDERER_URL'])
  } else {
    win.loadFile(join(__dirname, '../renderer/index.html'))
  }
}

app.whenReady().then(() => {
  startBackend()
  createWindow()
  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) createWindow()
  })
})

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit()
})

app.on('quit', () => {
  backend?.kill()
})
