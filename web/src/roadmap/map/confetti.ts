/**
 * Phân tán mảnh giấy khi đánh dấu xong 1 màn.
 *
 * Canvas 2D là nơi DUY NHẤT app vẽ pixel thật (ROADMAP-MAP-IDEA §1) — phần còn
 * lại của bản đồ là SVG để test được và không cần WebGL. Vì sinh lực là hàm
 * thuần nên kiểm tra được số mảnh, góc nảy, màu mà không cần mở trình duyệt.
 */

/** Bảng màu lấy từ chất liệu: Ember (nhấn), Jade (xong), Amber (sao), giấy. */
const COLORS = ['#c8502a', '#d99b23', '#2e6b57', '#f6f2e9', '#fffdf8'];

export interface ConfettiPiece {
  x: number;
  y: number;
  /** px/giây. */
  vx: number;
  vy: number;
  /** Bán kính/chiều rộng hình, px. */
  size: number;
  /** radian, quay mỗi giây. */
  spin: number;
  angle: number;
  color: string;
  /** Hình: 0 = chữ nhật, 1 = tròn. Trộn 2 hình cho mảnh không đều. */
  round: boolean;
  /** Giây còn sống — hết hạn thì mảnh rụng khỏi vùng nhìn. */
  life: number;
}

export interface BurstOptions {
  count?: number;
  /** Hạt giống — cùng hạt phải cho cùng vụ nổ để test so được. */
  seed?: number;
  width: number;
  height: number;
}

export function burst({ count = 48, seed = 1, width, height }: BurstOptions): ConfettiPiece[] {
  const pieces: ConfettiPiece[] = [];
  for (let i = 0; i < count; i += 1) {
    // Góc nảy chỉ lên trên 2 góc trên ⇒ mảnh văng ra như pháo giấy, không
    // như vụn nổ. `angle` đo từ trục X, phía trên là -90°.
    const spread = (Math.PI * 2) / 5;
    const base = -Math.PI / 2 - spread / 2;
    const angle = base + rnd(seed + i) * spread;
    const speed = 260 + rnd(seed + i + 911) * 420;
    pieces.push({
      x: width / 2,
      y: height / 2,
      vx: Math.cos(angle) * speed,
      vy: Math.sin(angle) * speed,
      size: 5 + rnd(seed + i + 37) * 7,
      spin: (rnd(seed + i + 71) - 0.5) * 12,
      angle: rnd(seed + i + 131) * Math.PI * 2,
      color: COLORS[(seed + i) % COLORS.length],
      round: rnd(seed + i + 53) > 0.65,
      life: 1.5 + rnd(seed + i + 199) * 0.9,
    });
  }
  return pieces;
}

/** Bước 1 khung hình. Trả mảnh MỚI đã cập nhật; mảnh hết đời bị loại. */
export function step(pieces: readonly ConfettiPiece[], dt: number): ConfettiPiece[] {
  const out: ConfettiPiece[] = [];
  for (const p of pieces) {
    const life = p.life - dt;
    if (life <= 0) continue;
    out.push({
      ...p,
      x: p.x + p.vx * dt,
      y: p.y + p.vy * dt,
      vx: p.vx,
      vy: p.vy + 900 * dt,
      angle: p.angle + p.spin * dt,
      life,
    });
  }
  return out;
}

function rnd(seed: number): number {
  const x = Math.sin(seed * 12.9898) * 43758.5453;
  return x - Math.floor(x);
}
