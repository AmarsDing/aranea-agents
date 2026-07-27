#!/usr/bin/env node
/**
 * tts-edge.mjs · 微软 Edge TTS（msedge-tts）
 *
 * 用法：
 *   node tts-edge.mjs --text "你好" --out demo.mp3
 *   node tts-edge.mjs --text-file script.txt --out out.mp3 --speed 1.0 --voice zh-CN-YunjianNeural
 *
 * 输出：
 *   - mp3 文件写到 --out 路径
 *   - stdout 打印一行 JSON: {"path":"...","duration":12.34,"bytes":54321}
 *
 * 依赖：Node 18+、msedge-tts、ffmpeg
 */

import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { MsEdgeTTS, OUTPUT_FORMAT } from 'msedge-tts';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

function getDuration(filePath) {
  try {
    const ffprobe = process.env.FFPROBE_PATH || 'ffprobe';
    const out = execFileSync(ffprobe, [
      '-v', 'error',
      '-show_entries', 'format=duration',
      '-of', 'default=noprint_wrappers=1:nokey=1',
      filePath,
    ], { encoding: 'utf8' });
    return parseFloat(out.trim());
  } catch (e) {
    return null;
  }
}

function parseArgs(argv) {
  const args = { speed: '1.0', encoding: 'mp3' };
  for (let i = 2; i < argv.length; i++) {
    const a = argv[i];
    if (a === '--text') args.text = argv[++i];
    else if (a === '--text-file') args.textFile = argv[++i];
    else if (a === '--out') args.out = argv[++i];
    else if (a === '--speed') args.speed = argv[++i];
    else if (a === '--voice') args.voice = argv[++i];
    else if (a === '--encoding') args.encoding = argv[++i];
    else if (a === '--help' || a === '-h') args.help = true;
  }
  return args;
}

function usage() {
  console.error(`
tts-edge.mjs · 微软 Edge TTS

  --text <str>          要合成的文本
  --text-file <path>    从文件读取文本（与 --text 二选一）
  --out <path>          输出 mp3 路径（必填）
  --speed <float>       语速倍率，默认 1.0（0.5-2.0）
  --voice <voice_id>    音色，默认 zh-CN-YunjianNeural
  --encoding <ext>      仅兼容，统一输出 mp3
`.trim());
  process.exit(1);
}

async function main() {
  const args = parseArgs(process.argv);
  if (args.help) usage();

  let text = args.text;
  if (!text && args.textFile) {
    text = fs.readFileSync(args.textFile, 'utf8').trim();
  }
  if (!text) { console.error('错：缺 --text 或 --text-file'); usage(); }
  if (!args.out) { console.error('错：缺 --out'); usage(); }

  const outPath = path.resolve(args.out);
  fs.mkdirSync(path.dirname(outPath), { recursive: true });

  const voice = args.voice || 'zh-CN-YunjianNeural';
  const speed = parseFloat(args.speed) || 1.0;
  const ratePercent = Math.round((speed - 1) * 100);
  const rateStr = ratePercent >= 0 ? `+${ratePercent}%` : `${ratePercent}%`;

  // msedge-tts 偶发 websocket 挂死：超时 + 重试
  const TIMEOUT_MS = 60_000;
  const MAX_TRIES = 3;
  let audioFilePath = null;
  let lastErr = null;
  for (let attempt = 1; attempt <= MAX_TRIES; attempt++) {
    const tts = new MsEdgeTTS();
    try {
      // setMetadata 也可能挂死（websocket 建连阶段），同样纳入超时
      await Promise.race([
        tts.setMetadata(voice, OUTPUT_FORMAT.AUDIO_24KHZ_96KBITRATE_MONO_MP3),
        new Promise((_, reject) =>
          setTimeout(() => reject(new Error(`TTS setMetadata 超时（${TIMEOUT_MS / 1000}s）`)), TIMEOUT_MS),
        ),
      ]);
      const result = await Promise.race([
        tts.toFile(path.dirname(outPath), text, {
          rate: rateStr,
          pitch: 'default',
          volume: 'default',
        }),
        new Promise((_, reject) =>
          setTimeout(() => reject(new Error(`TTS 超时（${TIMEOUT_MS / 1000}s）`)), TIMEOUT_MS),
        ),
      ]);
      audioFilePath = result.audioFilePath;
      break;
    } catch (e) {
      lastErr = e;
      console.error(`第 ${attempt}/${MAX_TRIES} 次合成失败：${e.message}`);
      try { tts.close && tts.close(); } catch { /* ignore */ }
    }
  }
  if (!audioFilePath) {
    throw lastErr || new Error('TTS 多次重试仍失败');
  }

  // 由于 msedge-tts 的 mp3 内部编码可能与 ffmpeg concat 不兼容，统一转码
  const ffmpeg = process.env.FFMPEG_PATH || 'ffmpeg';
  const tmpPath = outPath + '.tmp.mp3';
  execFileSync(ffmpeg, [
    '-y', '-i', audioFilePath,
    '-ar', '24000', '-ac', '1', '-q:a', '2',
    '-f', 'mp3', tmpPath,
  ], { stdio: ['ignore', 'pipe', 'pipe'] });

  fs.renameSync(tmpPath, outPath);
  // 清理原始输出
  if (audioFilePath !== outPath && fs.existsSync(audioFilePath)) {
    fs.unlinkSync(audioFilePath);
  }

  const duration = getDuration(outPath);
  const result = { path: outPath, duration, bytes: fs.statSync(outPath).size, text_chars: text.length };
  console.log(JSON.stringify(result));
  // msedge-tts 可能残留未关闭的 WebSocket，导致进程不退、父进程 execFileSync 挂死
  process.exit(0);
}

main().catch((err) => {
  console.error(`TTS 失败：${err.message}`);
  process.exit(1);
});
