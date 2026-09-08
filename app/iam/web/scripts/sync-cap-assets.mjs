import { copyFile, mkdir } from "node:fs/promises"
import { createRequire } from "node:module"
import { dirname, join } from "node:path"
import { fileURLToPath } from "node:url"

const require = createRequire(import.meta.url)
const packageRoot = dirname(require.resolve("@cap.js/wasm/package.json"))
const webRoot = join(dirname(fileURLToPath(import.meta.url)), "..")
const destination = join(webRoot, "public", "cap-assets")

await mkdir(destination, { recursive: true })
await copyFile(
  join(packageRoot, "browser", "cap_wasm_bg.wasm"),
  join(destination, "cap_wasm_bg.wasm")
)
