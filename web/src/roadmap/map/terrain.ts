/**
 * 6 địa hình của bản đồ: màu, hình nền, nhãn tiếng Việt.
 *
 * Tên địa hình lấy từ `MapTerrain` ở `graphql/operations.ts` — danh sách DUY
 * NHẤT, khớp `enum Terrain` ở `api/graph/schema/common.graphqls` và whitelist
 * `domain/roadmap.AllTerrains` + CHECK migration `00004`. File này chỉ gắn
 * chất liệu hình ảnh, không khai lại danh sách.
 *
 * ── Biên độ KHÔNG ở đây ──────────────────────────────────────────────────
 * Toạ độ node do server tính (`domain/roadmap.ComputeLayout`, viewBox
 * 0 0 1000 2000) rồi đóng gói vào `Topic.point`. `terrain.ts` chỉ vẽ — tính
 * lại toạ độ ở client là 2 bản luật layout trôi khỏi nhau.
 */

import type { MapDirection, MapTerrain } from '../../graphql/operations';

/**
 * Chiều đi của bản đồ: `up` cuộn dọc, `right` cuộn ngang.
 *
 * TRƯỚC ĐÂY file này khai `DIRECTIONS = ['UP', 'RIGHT'] as const` và suy ra
 * `Direction` từ nó. Đó là **nguồn sự thật thứ 2** chết: không consumer nào
 * ngoài test dùng tới mảng đó, và nếu thêm hướng thứ 3 vào
 * `MapDirection` (`graphql/operations.ts`) mà quên cập nhật `DIRECTIONS` thì
 * không gì báo đỏ. Nay `Direction` trỏ thẳng về `MapDirection` — cùng cách
 * `Terrain = MapTerrain` ngay dưới, và cùng lý do: danh sách DUY NHẤT ở
 * `graphql/operations.ts`.
 *
 * Hệ quả đã chấp nhận: không còn danh sách hướng ở tầng runtime, nên
 * `vitest` không assert được "đúng 2 hướng" (union kiểu TS không liệt kê được
 * lúc chạy). Thứ khoá phần đó bây giờ là `vue-tsc` + bảng `HORIZONTAL` bên
 * dưới khai `Record<Direction, boolean>` — thêm hướng thứ 3 là ĐỎ.
 */
export type Direction = MapDirection;

export interface TerrainSkin {
  /** Nhãn tiếng Việt hiện trên chip chọn bản đồ. */
  label: string;
  /** Nền trời (đỉnh gradient). */
  sky: string;
  /** Nền đất (đáy gradient). */
  ground: string;
  /** Màu đường đi. */
  path: string;
  /** Màu viền sáng của node. */
  rim: string;
}

/**
 * Chất liệu hình ảnh cho 6 địa hình.
 *
 * TÊN ở khoá là danh sách DUY NHẤT ở `graphql/operations.ts` (`MapTerrain`) —
 * file này chỉ gắn màu/nhãn cho chúng, không khai lại danh sách. `Record<
 * MapTerrain, TerrainSkin>` là CÁI CỚ để `vue-tsc` bắt lỗi khi 2 bên lệch: thêm
 * 1 địa hình vào `MapTerrain` mà quên dòng ở đây ⇒ đỏ ngay.
 *
 * Trước M7a có 2 danh sách độc lập (`TERRAINS` ở đây + `MapTerrain` ở
 * `operations.ts`) và 1 bảng tra 6 tên hoàn toàn khác server
 * (`PLAIN/FOREST/HILL/MOUNTAIN/WATER/DESERT`), nên UI vẽ sai chất liệu mà
 * không tín hiệu gì — cùng lớp lỗi "2 nguồn sự thật" mà `enum Terrain` mắc ở
 * server.
 */
export const TERRAIN_SKIN: Record<MapTerrain, TerrainSkin> = {
  MEADOW: { label: 'Đồng cỏ', sky: '#20361f', ground: '#12200f', path: '#9fce7c', rim: '#dff0c6' },
  DESERT: { label: 'Sa mạc', sky: '#4a3418', ground: '#20150a', path: '#e5bb72', rim: '#fbe6bd' },
  SNOW: { label: 'Tuyết', sky: '#28323f', ground: '#141a22', path: '#cfe0ee', rim: '#f2f8fd' },
  VOLCANO: { label: 'Núi lửa', sky: '#40201b', ground: '#180a09', path: '#e08a63', rim: '#fbd8c1' },
  OCEAN: { label: 'Biển', sky: '#173a4c', ground: '#081c28', path: '#84c7e4', rim: '#d6f0fb' },
  CITY: { label: 'Thành phố', sky: '#1c3630', ground: '#0d1d1a', path: '#7fc4a3', rim: '#d3efe0' },
};

/** Tên địa hình — CÙNG kiểu với `MapTerrain`, để code đọc bản đồ không cần import
 * từ `graphql/`. */
export type Terrain = MapTerrain;

/** 6 địa hình theo thứ tự hiển thị trên chip chọn bản đồ, suy ra từ `TERRAIN_SKIN`. */
export const TERRAINS = Object.keys(TERRAIN_SKIN) as readonly Terrain[];

/** Chữ ngắn để vẽ mảnh nền phụ. */
export function terrainLabel(t: Terrain): string {
  return TERRAIN_SKIN[t]?.label ?? 'Đồng cỏ';
}

/**
 * Trục cuộn của TỪNG hướng. `Record<Direction, boolean>` là CÁI CỚ để
 * `vue-tsc` bắt lỗi: thêm hướng thứ 3 vào `MapDirection` mà quên dòng ở đây
 * ⇒ đỏ ngay, thay vì hướng mới rơi về `false` trong im lặng.
 */
const HORIZONTAL: Record<Direction, boolean> = { UP: false, RIGHT: true };

/** `right` = cuộn ngang; mọi thứ khác = `up`. Dùng cho khoá trục scroll. */
export function isHorizontal(d: Direction | null | undefined): boolean {
  return d != null && HORIZONTAL[d];
}

/**
 * Số ngẫu nhiên có hạt giống — `mulberry32`. Vì sao không `Math.random`: hạt
 * nền phải GIỐNG NHAU ở mọi lần render, nếu không terrain nhấp nháy mỗi lần
 * component re-render và `node` nào cũng không test được.
 */
export function seeded(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

/** Hình dạng mép trên của 1 dải nền phụ. */
type BandShape = 'roll' | 'spike' | 'smooth' | 'wave';

/** Mỗi địa hình dùng 2–3 dải nền, dải sau mờ dần để tạo chiều sâu. */
const BANDS: Record<Terrain, Array<{ shape: BandShape; base: number; amp: number; step: number }>> = {
  MEADOW: [
    { shape: 'roll', base: 0.72, amp: 0.045, step: 260 },
    { shape: 'roll', base: 0.86, amp: 0.03, step: 200 },
  ],
  CITY: [
    { shape: 'spike', base: 0.7, amp: 0.075, step: 120 },
    { shape: 'spike', base: 0.88, amp: 0.05, step: 90 },
  ],
  SNOW: [
    { shape: 'roll', base: 0.66, amp: 0.075, step: 340 },
    { shape: 'smooth', base: 0.86, amp: 0.04, step: 260 },
  ],
  VOLCANO: [
    { shape: 'spike', base: 0.62, amp: 0.15, step: 220 },
    { shape: 'smooth', base: 0.9, amp: 0.05, step: 180 },
  ],
  OCEAN: [
    { shape: 'wave', base: 0.74, amp: 0.022, step: 300 },
    { shape: 'wave', base: 0.9, amp: 0.016, step: 200 },
  ],
  DESERT: [
    { shape: 'smooth', base: 0.7, amp: 0.06, step: 380 },
    { shape: 'smooth', base: 0.9, amp: 0.035, step: 240 },
  ],
};

export interface DecorBand {
  d: string;
  /** 0.18 → 0.5: dải càng gần camera càng đậm. */
  opacity: number;
}

/**
 * Dựng 2–3 dải nền cho 1 bản đồ. `seed` là số thứ tự stage (0, 1, 2…) để
 * mỗi chặng có hình khác nhau nhưng vẫn tất định.
 */
export function terrainBands(terrain: Terrain, seed: number, width: number, height: number): DecorBand[] {
  const rnd = seeded(seed * 7919 + 17);
  return (BANDS[terrain] ?? BANDS.MEADOW).map((band, index) => {
    const floor = height * band.base;
    const amp = height * band.amp;
    const step = band.step;
    const pts: Array<[number, number]> = [];
    for (let x = -step; x <= width + step; x += step) {
      const phase = (x / step) * Math.PI * 2 + rnd() * 0.6;
      const y =
        band.shape === 'wave'
          ? floor + Math.sin(phase) * amp
          : band.shape === 'roll'
            ? floor + Math.sin(phase) * amp + (rnd() - 0.5) * amp
            : band.shape === 'spike'
              ? floor + (rnd() - 0.5) * amp * 2
              : floor + Math.sin(phase * 0.5) * amp;
      pts.push([x, y]);
    }
    return {
      d: bandPath(pts, band.shape === 'spike' ? 'line' : 'smooth', height),
      opacity: 0.5 - index * 0.18,
    };
  });
}

/** Nối các điểm thành 1 đường kín khung. `smooth` = bo qua điểm giữa. */
function bandPath(pts: Array<[number, number]>, mode: 'line' | 'smooth', height: number): string {
  const head = pts[0];
  let d = `M ${head[0].toFixed(1)} ${head[1].toFixed(1)}`;
  for (let i = 1; i < pts.length; i += 1) {
    const [x, y] = pts[i];
    if (mode === 'line') {
      d += ` L ${x.toFixed(1)} ${y.toFixed(1)}`;
      continue;
    }
    const [px, py] = pts[i - 1];
    const mx = (px + x) / 2;
    const my = (py + y) / 2;
    d += ` Q ${px.toFixed(1)} ${py.toFixed(1)} ${mx.toFixed(1)} ${my.toFixed(1)}`;
  }
  const last = pts[pts.length - 1];
  return `${d} L ${last[0].toFixed(1)} ${height} L ${pts[0][0].toFixed(1)} ${height} Z`;
}
