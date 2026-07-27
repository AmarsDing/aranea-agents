#!/usr/bin/env node
/**
 * timeline-to-srt.mjs · 把 narrate-pipeline 的 timeline.json 转成剪映可导入的 SRT 字幕
 *
 * 用法：node timeline-to-srt.mjs --timeline <timeline.json> --out <subtitles.srt> [--max-chars 16]
 *
 * 说明：
 *   - 以 chunk 为单位生成字幕条目（chunk 是解说稿中按 [[cue:xx]] 切分的句子）
 *   - 过长的 chunk 按标点拆成多条，时间按字数比例分配
 *   - 时间格式：HH:MM:SS,mmm（SRT 标准，逗号分隔毫秒）
 */

import fs from 'node:fs';
import path from 'node:path';

function parseArgs(argv) {
  const args = { maxChars: '16' };
  for (let i = 2; i < argv.length; i++) {
    const a = argv[i];
    if (a === '--timeline') args.timeline = argv[++i];
    else if (a === '--out') args.out = argv[++i];
    else if (a === '--max-chars') args.maxChars = argv[++i];
  }
  return args;
}

function fmtTime(sec) {
  const ms = Math.round(sec * 1000);
  const h = String(Math.floor(ms / 3600000)).padStart(2, '0');
  const m = String(Math.floor((ms % 3600000) / 60000)).padStart(2, '0');
  const s = String(Math.floor((ms % 60000) / 1000)).padStart(2, '0');
  const mmm = String(ms % 1000).padStart(3, '0');
  return `${h}:${m}:${s},${mmm}`;
}

/** 按标点把长句拆成 ≤maxChars 的短句：英文/数字 token 不拆开，标点永远跟随前句 */
function splitText(text, maxChars) {
  const clean = text.replace(/\s*\n\s*/g, '').trim();
  if (clean.length <= maxChars) return [clean];

  // 分词：连续英文/数字/符号（如 Aranea-Agents、AI、EP1）视为一个整体 token
  const tokens = clean.match(/[A-Za-z0-9._-]+|[\s\S]/g) || [];
  const isPunct = (t) => /^[，。；、！？：,.!?;:—…「」『』（）()]$/.test(t);
  const endsWithBreak = (s) => /[，。；、！？：,.!?;:—…」』）)]$/.test(s);

  const parts = [];
  let buf = '';
  const pushBuf = () => { if (buf) { parts.push(buf); buf = ''; } };

  for (const tok of tokens) {
    // 加上这个 token 会超长且 buf 已过半 → 先断（标点除外，标点必须跟随前句）
    if (!isPunct(tok) && buf && buf.length + tok.length > maxChars && buf.length >= maxChars * 0.5) {
      pushBuf();
    }
    buf += tok;
    // 在标点处且长度已够 → 自然断点
    if (buf.length >= maxChars * 0.6 && endsWithBreak(buf)) {
      pushBuf();
    }
  }
  pushBuf();

  // 纯标点片段并回前一条
  const merged = [];
  for (const p of parts) {
    if (merged.length && /^[，。；、！？：,.!?;:—…」』）)]+$/.test(p)) {
      merged[merged.length - 1] += p;
    } else {
      merged.push(p);
    }
  }
  return merged;
}

function main() {
  const args = parseArgs(process.argv);
  if (!args.timeline || !args.out) {
    console.error('用法：node timeline-to-srt.mjs --timeline <timeline.json> --out <subtitles.srt> [--max-chars 16]');
    process.exit(1);
  }
  const maxChars = parseInt(args.maxChars, 10) || 16;
  const timeline = JSON.parse(fs.readFileSync(args.timeline, 'utf8'));

  // chunks 嵌套在 scenes[].chunks 中，absoluteStart/absoluteEnd 为整轨时间
  const allChunks = (timeline.scenes || []).flatMap((s) => s.chunks || []);

  const entries = [];
  let idx = 1;
  for (const chunk of allChunks) {
    const text = (chunk.text || '').trim();
    if (!text) continue;
    const start = chunk.absoluteStart;
    const end = chunk.absoluteEnd;
    const segs = splitText(text, maxChars);
    if (segs.length === 1) {
      entries.push({ idx: idx++, start, end, text: segs[0] });
    } else {
      const totalChars = segs.reduce((n, s) => n + s.length, 0);
      let cursor = start;
      for (const seg of segs) {
        const segDur = ((end - start) * seg.length) / totalChars;
        entries.push({ idx: idx++, start: cursor, end: cursor + segDur, text: seg });
        cursor += segDur;
      }
    }
  }

  const srt = entries
    .map((e) => `${e.idx}\n${fmtTime(e.start)} --> ${fmtTime(e.end)}\n${e.text}\n`)
    .join('\n');
  fs.mkdirSync(path.dirname(path.resolve(args.out)), { recursive: true });
  fs.writeFileSync(args.out, '﻿' + srt, 'utf8'); // BOM 让剪映/记事本正确识别 UTF-8
  console.log(JSON.stringify({ out: path.resolve(args.out), entries: entries.length, duration: timeline.totalDuration }));
}

main();
