const fs = require('fs')
const path = require('path')
const noticePath = path.join(__dirname, 'notice.md')
const outputPath = path.join(__dirname, 'src-vue', 'shared', 'notice.ts')
const content = fs.readFileSync(noticePath, 'utf8')
const escaped = content.replace(/\\/g, '\\\\').replace(/`/g, '\\`').replace(/\$/g, '\\$')
const ts = `export const noticeContent = \`${escaped}\`\n`
fs.writeFileSync(outputPath, ts, 'utf8')
console.log('[Notice] Compiled notice.md -> src-vue/shared/notice.ts')
