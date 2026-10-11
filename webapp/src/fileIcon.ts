import { createElement } from 'react'
import type { ReactElement } from 'react'
import {
  LuFile,
  LuFileArchive,
  LuFileAudio,
  LuFileChartPie,
  LuFileCode,
  LuFileCog,
  LuFileDigit,
  LuFileImage,
  LuFileJson,
  LuFileSpreadsheet,
  LuFileText,
  LuFileType,
  LuFileVideo,
  LuFolder,
} from 'react-icons/lu'
import type { IconType } from 'react-icons'

/**
 * 按扩展名挑图标。
 *
 * 只按扩展名判断，不看内容——网盘里的文件我们只能拿到名字，而且这里的目标是
 * 「一眼能分辨大概是什么」，不是精确的文件类型识别。认不出来就用通用文件图标。
 */
const BY_EXTENSION: Record<string, IconType> = {}

function register(Icon: IconType, extensions: readonly string[]): void {
  for (const ext of extensions) {
    BY_EXTENSION[ext] = Icon
  }
}

register(LuFileVideo, [
  'mp4', 'mkv', 'avi', 'mov', 'flv', 'wmv', 'webm', 'm4v', 'mpg', 'mpeg',
  'rmvb', 'rm', 'ts', 'vob', '3gp',
])

register(LuFileAudio, [
  'mp3', 'flac', 'wav', 'aac', 'ogg', 'oga', 'm4a', 'wma', 'ape', 'opus', 'mid',
])

register(LuFileImage, [
  'jpg', 'jpeg', 'png', 'gif', 'bmp', 'webp', 'svg', 'ico', 'tif', 'tiff',
  'heic', 'heif', 'raw', 'psd', 'ai',
])

register(LuFileArchive, [
  'zip', 'rar', '7z', 'tar', 'gz', 'tgz', 'bz2', 'xz', 'zst', 'lz', 'lzma',
  'cab', 'iso', 'img', 'dmg',
])

// 文档类。Lucide 没有专门的 pdf/word 图标，统一用「有文字的文件」。
register(LuFileText, ['txt', 'md', 'markdown', 'log', 'pdf', 'doc', 'docx', 'odt', 'rtf'])
register(LuFileSpreadsheet, ['xls', 'xlsx', 'csv', 'tsv', 'ods', 'numbers'])
register(LuFileChartPie, ['ppt', 'pptx', 'odp', 'key'])

register(LuFileCode, [
  'js', 'mjs', 'cjs', 'jsx', 'ts', 'tsx', 'py', 'go', 'java', 'kt', 'c', 'h',
  'cpp', 'cc', 'hpp', 'cs', 'rs', 'rb', 'php', 'swift', 'scala', 'lua', 'pl',
  'sh', 'bash', 'zsh', 'fish', 'ps1', 'bat', 'cmd', 'sql', 'vue', 'svelte',
  'html', 'htm', 'css', 'scss', 'sass', 'less', 'xml', 'toml', 'ini', 'conf', 'cfg',
])
register(LuFileJson, ['json', 'json5', 'yaml', 'yml'])

// 可执行文件与安装包：Lucide 没有二进制文件的图标，用「齿轮」表示可运行的东西。
register(LuFileCog, [
  'exe', 'msi', 'deb', 'rpm', 'apk', 'appimage', 'bin', 'run', 'jar', 'app', 'pkg',
])

register(LuFileType, ['ttf', 'otf', 'woff', 'woff2', 'eot'])
register(LuFileDigit, ['stl', 'step', 'stp', 'iges', 'igs', 'obj', '3mf', 'blend', 'gcode'])

/** 取文件名最后一段扩展名，小写。没有扩展名时返回空串。 */
function extensionOf(name: string): string {
  const dot = name.lastIndexOf('.')
  // 开头的点（.gitignore 这类）不算扩展名。
  if (dot <= 0 || dot === name.length - 1) return ''
  return name.slice(dot + 1).toLowerCase()
}

/** 按文件名（或目录）挑一个图标。 */
export function fileIcon(name: string, isDir = false): IconType {
  if (isDir) return LuFolder
  return BY_EXTENSION[extensionOf(name)] ?? LuFile
}

/**
 * 直接渲染出对应的图标元素。
 *
 * 用 createElement 而不是 `const Icon = fileIcon(...)` 再 `<Icon />`：后者会让
 * oxlint 的 react(static-components) 规则以为我们在 render 里现造组件。
 * 这里返回的其实是模块级的现成图标，不会有状态重置问题，但没必要跟规则较劲。
 */
export function renderFileIcon(
  name: string,
  isDir = false,
  className?: string,
): ReactElement {
  return createElement(fileIcon(name, isDir), { className, 'aria-hidden': true })
}
