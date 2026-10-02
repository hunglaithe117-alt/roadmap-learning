// 6 địa hình — bao gồm 2 điều kiện mà UI hỏng âm thầm nếu hỏng: mỗi terrain
// phải có màu riêng, và hình nền phải TẤT ĐỊNH (cùng seed ra cùng đường).
import { describe, expect, it } from 'vitest';
import {
  TERRAINS,
  TERRAIN_SKIN,
  isHorizontal,
  seeded,
  terrainBands,
  terrainLabel,
  type Direction,
} from './terrain';

describe('test_terrain_palette', () => {
  it('test_six_terrains_are_whitelisted', () => {
    expect(TERRAINS).toHaveLength(6);
  });

  it('test_terrain_skin_covers_exactly_the_server_enum', () => {
    // `TERRAIN_SKIN` được khai `Record<MapTerrain, TerrainSkin>` nên `vue-tsc`
    // đã bắt lệch 1 chiều (thêm tên mà thiếu skin). NHƯNG `vitest` KHÔNG
    // typecheck — chạy `vitest` một mình thì lệch vẫn lọt. Test này chặn nốt
    // chiều còn lại: có skin thừa mà `MapTerrain` không khai.
    //
    // Danh sách lặp ở đây CỐ Ý: nó là kỳ vọng độc lập, đúng vai trò của
    // `enumSpecs` bên `api/`. Gộp 2 danh sách lại thì test này thành vô nghĩa.
    const serverEnum = ['MEADOW', 'DESERT', 'SNOW', 'VOLCANO', 'OCEAN', 'CITY'] as const;
    expect([...TERRAINS].sort()).toEqual([...serverEnum].sort());
    expect(Object.keys(TERRAIN_SKIN).sort()).toEqual([...serverEnum].sort());
  });

  // ── THỨ TỰ LÀ HỢP ĐỒNG UI, KHÔNG PHẢI THỨ TỰ NGẪU NHIÊN ──────────────
  //
  // Test trên so `.sort()` nên nó chỉ chứng minh "đủ 6 tên, đúng tập" — ĐẢO
  // THỨ TỰ mảng mà test vẫn xanh. Nhưng thứ tự trong `TERRAIN_SKIN` quyết
  // định thứ tự hiển thị trên chip chọn bản đồ: `StageForm.vue` render
  // `<option v-for="t in TERRAINS">`, và `Object.keys()` giữ thứ tự khai.
  //
  // ⇒ `MEADOW DESERT SNOW VOLCANO OCEAN CITY` là HỢP ĐỒNG: "Đồng cỏ" là
  // lựa chọn mặc định hợp lý nhất nên đứng đầu, "Thành phố" (mức cao nhất)
  // đứng cuối. Ai đổi thứ tự trong code phải sửa test này CÙNG LÚC, và phải
  // nói rõ vì sao — đó là ý nghĩa của việc test đỏ.
  it('test_terrains_order_is_the_ui_chip_order', () => {
    const chipOrder = ['MEADOW', 'DESERT', 'SNOW', 'VOLCANO', 'OCEAN', 'CITY'];
    expect([...TERRAINS]).toEqual(chipOrder);
    // `TERRAINS` suy ra từ `Object.keys(TERRAIN_SKIN)` nên phải khoá cả nguồn,
    // không chỉ kết quả — nếu không, đổi thứ tự khai ở `TERRAIN_SKIN` có thể
    // làm `TERRAINS` khác mà test này vẫn xanh (nó sẽ đỏ, nhưng phải đỏ ở đúng
    // chỗ để báo lệch là thứ tự).
    expect(Object.keys(TERRAIN_SKIN)).toEqual(chipOrder);
  });

  it('test_every_terrain_has_a_skin_with_five_colours', () => {
    for (const t of TERRAINS) {
      const skin = TERRAIN_SKIN[t];
      expect(skin, `thiếu skin cho ${t}`).toBeTruthy();
      for (const key of ['sky', 'ground', 'path', 'rim', 'label'] as const) {
        expect(skin[key], `${t}.${key} rỗng`).not.toBe('');
      }
    }
  });

  it('test_path_colour_differs_between_terrains', () => {
    // 6 terrain trùng màu đường đi = bản đồ 6 chặng nhìn như 1.
    const paths = new Set(TERRAINS.map((t) => TERRAIN_SKIN[t].path));
    expect(paths.size).toBe(TERRAINS.length);
  });

  it('test_every_terrain_has_a_vietnamese_label', () => {
    const labels = TERRAINS.map(terrainLabel);
    expect(new Set(labels).size).toBe(6);
    expect(labels.every((l) => /[a-zà-ỹ]/i.test(l))).toBe(true);
  });

  it('test_unknown_terrain_falls_back_instead_of_crashing', () => {
    expect(terrainLabel('SWAMP' as never)).toBe('Đồng cỏ');
  });
});

describe('test_direction', () => {
  it('test_only_right_is_horizontal', () => {
    expect(isHorizontal('RIGHT')).toBe(true);
    expect(isHorizontal('UP')).toBe(false);
    expect(isHorizontal(null)).toBe(false);
    expect(isHorizontal(undefined)).toBe(false);
  });

  // Trước M7b có `expect(DIRECTIONS).toHaveLength(2)` — một khẳng định vô
  // nghĩa trên 1 export chết (`DIRECTIONS` không consumer nào ngoài chính test
  // này dùng), và nó cũng là "nguồn sự thật thứ 2" cho danh sách hướng.
  //
  // Nay `Direction = MapDirection` (`graphql/operations.ts`) và bảng
  // `HORIZONTAL: Record<Direction, boolean>` trong `terrain.ts` khoá tập hướng
  // ở TẦNG TYPE — thêm hướng thứ 3 là `vue-tsc` đỏ. Danh sách dưới đây lặp
  // lại 2 hướng như một kỳ vọng độc lập (cùng vai trò với `serverEnum` ở
  // `test_terrain_palette`) và chứng minh đúng 1 hướng cuộn ngang.
  it('test_exactly_one_of_two_directions_is_horizontal', () => {
    const allDirections: readonly Direction[] = ['UP', 'RIGHT'];
    expect(allDirections.filter(isHorizontal)).toEqual(['RIGHT']);
  });
});

describe('test_seeded_random_is_deterministic', () => {
  it('test_same_seed_yields_same_sequence', () => {
    const a = seeded(42);
    const b = seeded(42);
    expect([a(), a(), a()]).toEqual([b(), b(), b()]);
  });

  it('test_values_stay_inside_unit_interval', () => {
    const r = seeded(7);
    for (let i = 0; i < 50; i += 1) {
      const v = r();
      expect(v).toBeGreaterThanOrEqual(0);
      expect(v).toBeLessThan(1);
    }
  });
});

describe('test_terrain_bands', () => {
  it('test_bands_are_stable_across_renders', () => {
    const a = terrainBands('VOLCANO', 3, 1000, 2000);
    const b = terrainBands('VOLCANO', 3, 1000, 2000);
    expect(a.map((x) => x.d)).toEqual(b.map((x) => x.d));
  });

  it('test_different_seeds_give_different_backdrops', () => {
    const a = terrainBands('DESERT', 1, 1000, 2000);
    const b = terrainBands('DESERT', 2, 1000, 2000);
    expect(a.map((x) => x.d)).not.toEqual(b.map((x) => x.d));
  });

  it('test_unknown_terrain_falls_back_to_a_plain_backdrop', () => {
    expect(terrainBands('SWAMP' as never, 1, 1000, 2000)).toEqual(terrainBands('MEADOW', 1, 1000, 2000));
  });

  it('test_band_paths_close_to_the_ground', () => {
    for (const band of terrainBands('OCEAN', 0, 1000, 2000)) {
      expect(band.d.endsWith('Z'), 'dải nền phải khép kín').toBe(true);
    }
  });
});
