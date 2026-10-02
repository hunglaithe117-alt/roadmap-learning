<!--
  Confetti Canvas 2D — lớp vẽ pixel DUY NHẤT của bản đồ (ROADMAP-MAP-IDEA §1).
  Mọi thứ còn lại là SVG để test được trong vitest và không cần WebGL.

  KHÔNG render gì khi `confettiAllowed()` false: hệ điều hành bảo giảm chuyển
  động, hoặc người dùng tắt ở Cài đặt. Cả hai điều kiện phải cho phép — đây là
  yêu cầu bắt buộc §6, nên phần hiện thực cũng chỉ có 1 nhánh thoát sớm.
-->
<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue';
import { burst, step, type ConfettiPiece } from './confetti';
import { confettiAllowed } from './viewPrefs';

const props = defineProps<{ /** Đếm lần mỗi node vừa đánh dấu Xong. */ trigger: number; seed?: number }>();

const canvas = ref<HTMLCanvasElement | null>(null);
let pieces: ConfettiPiece[] = [];
let frame = 0;
let last = 0;

function stop(): void {
  if (frame) cancelAnimationFrame(frame);
  frame = 0;
  pieces = [];
}

function paint(): void {
  const el = canvas.value;
  const ctx = el?.getContext('2d');
  if (!el || !ctx) return;
  ctx.clearRect(0, 0, el.width, el.height);
  for (const p of pieces) {
    ctx.save();
    ctx.translate(p.x, p.y);
    ctx.rotate(p.angle);
    ctx.globalAlpha = Math.max(0, Math.min(1, p.life));
    ctx.fillStyle = p.color;
    if (p.round) {
      ctx.beginPath();
      ctx.arc(0, 0, p.size / 2, 0, Math.PI * 2);
      ctx.fill();
    } else {
      ctx.fillRect(-p.size / 2, -p.size / 3, p.size, p.size * 0.66);
    }
    ctx.restore();
  }
}

function loop(now: number): void {
  const dt = Math.min((now - last) / 1000, 0.05);
  last = now;
  pieces = step(pieces, dt);
  paint();
  frame = pieces.length > 0 ? requestAnimationFrame(loop) : 0;
}

function fire(): void {
  const el = canvas.value;
  if (!el || !confettiAllowed()) return;
  const box = el.getBoundingClientRect();
  el.width = Math.max(1, Math.round(box.width));
  el.height = Math.max(1, Math.round(box.height));
  pieces = burst({
    seed: props.seed ?? props.trigger,
    width: el.width,
    height: el.height,
  });
  last = performance.now();
  stop();
  frame = requestAnimationFrame(loop);
}

watch(() => props.trigger, fire);
onBeforeUnmount(stop);

defineExpose({ pieces: () => pieces, stop });
</script>

<template>
  <canvas
    v-if="confettiAllowed()"
    ref="canvas"
    data-testid="confetti"
    class="pointer-events-none absolute inset-0 h-full w-full"
    aria-hidden="true"
  />
</template>
