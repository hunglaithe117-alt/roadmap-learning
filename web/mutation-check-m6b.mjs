// Mutation-check M6b — sửa ngược từng quyết định, test phải ĐỎ, rồi KHÔI PHỤC.
//
// Cách làm giống M3/M4/M5: chỉ tin kết luận khi số đo nhất quán. Mỗi mutation
// copy file ra `.bak` TRƯỚC, sửa, chạy test, rồi copy lại — không làm thao tác
// "revert bằng thay thế chuỗi" vì một mutation có thể làm chuỗi đó xuất hiện ở
// nhiều chỗ và hỏng file (đã xảy ra 1 lần: chuỗi rỗng được chèn vào mọi ký tự).
import { copyFileSync, readFileSync, writeFileSync } from 'node:fs';
import { execSync } from 'node:child_process';

const cases = [
  {
    name: 'fetchPath bỏ sortPathTree (F8 của M4 quay lại)',
    file: 'src/roadmap/api.ts',
    from: '  return sortPathTree(path);',
    to: '  return path;',
    tests: 'src/roadmap/api.test.ts',
  },
  {
    name: 'LevelPanel bỏ luật trục (vuốt ngang ăn mất thao tác hoàn thành)',
    file: 'src/roadmap/map/LevelPanel.vue',
    from: "  if (!isSwipe({ dx, dy }, 'y')) return;",
    to: '  if (Math.max(Math.abs(dx), Math.abs(dy)) < 80) return;',
    tests: 'src/roadmap/map/MapCanvas.test.ts',
  },
  {
    name: 'MapCanvas bỏ khoá trục phụ (vuốt chéo cuộn bản đồ)',
    file: 'src/roadmap/map/MapCanvas.vue',
    from: "  if (axis !== (horizontal.value ? 'x' : 'y')) return;",
    to: '  // mutation: bỏ khoá trục phụ',
    tests: 'src/roadmap/map/MapCanvas.test.ts',
  },
  {
    name: 'confettiAllowed bỏ qua prefers-reduced-motion',
    file: 'src/roadmap/map/viewPrefs.ts',
    from: '  return readConfettiEnabled() && !prefersReducedMotion();',
    to: '  return readConfettiEnabled();',
    tests: 'src/roadmap/map/viewPrefs.test.ts src/routes/RoadmapMap.test.ts',
  },
  {
    name: 'ẩn nút Đánh dấu Xong (vuốt thành đường duy nhất)',
    file: 'src/roadmap/map/LevelPanel.vue',
    from: '<Button v-if="!done" data-testid="mark-done"',
    to: '<Button v-if="false" data-testid="mark-done"',
    tests: 'src/roadmap/map/MapCanvas.test.ts',
  },
  {
    name: 'nút /review không phụ thuộc deckId',
    file: 'src/roadmap/map/LevelPanel.vue',
    from: 'v-if="deckId"\n        data-testid="goto-review"',
    to: 'data-testid="goto-review"',
    tests: 'src/roadmap/map/MapCanvas.test.ts',
  },
  {
    name: 'bỏ chế độ danh sách dự phòng',
    file: 'src/routes/RoadmapMap.vue',
    from: "  view.value = route.query.view === 'list' ? 'list' : route.query.view === 'map' ? 'map' : readView();",
    to: '  view.value = readView();',
    tests: 'src/routes/RoadmapMap.test.ts',
  },
  {
    name: 'lọc tag bookmark thành so chuỗi con ở client',
    file: 'src/bookmarks/api.ts',
    from: `  const { bookmarks } = await runQuery(Bookmarks, {
    status: filter.status ?? null,
    tag: filter.tag ?? null,
  });
  return bookmarks;`,
    to: `  const { bookmarks } = await runQuery(Bookmarks, { status: filter.status ?? null, tag: null });
  const want = (filter.tag ?? '').trim().toLowerCase();
  return want === '' ? bookmarks : bookmarks.filter((b) => b.tagList.some((t) => t.includes(want)));`,
    tests: 'src/routes/Bookmarks.test.ts',
  },
  {
    name: 'nút nạp HSK không gọi gì cả',
    file: 'src/routes/ZhPinyin.vue',
    from: '<Button size="sm" :disabled="importing" @click="importHsk">',
    to: '<Button size="sm" :disabled="true" @click="() => {}">',
    tests: 'src/routes/RestoredFeatures.test.ts',
  },
  {
    name: 'ErrorBook bỏ cardId khỏi query',
    file: 'src/routes/ErrorBook.vue',
    from: '      fetchErrors(activeCardId.value, 50),',
    to: '      fetchErrors(undefined, 50),',
    tests: 'src/routes/RestoredFeatures.test.ts',
  },
  {
    name: 'chế độ cả vòng ghi SRS từng câu (mất vòng)',
    file: 'src/routes/ZhPinyin.vue',
    from: "    if (mode.value === 'card') {",
    to: '    if (true) {',
    tests: 'src/routes/RestoredFeatures.test.ts',
  },
  {
    name: 'node locked bấm được',
    file: 'src/roadmap/map/MapCanvas.vue',
    from: "@click.stop=\"isPlayable(n.level) && emit('select', n.id)\"",
    to: "@click.stop=\"emit('select', n.id)\"",
    tests: 'src/roadmap/map/MapCanvas.test.ts',
  },
  {
    name: 'createBookmark quên afterMutation()',
    file: 'src/bookmarks/api.ts',
    from: '  await afterMutation();\n  return data.createBookmark.bookmark;',
    to: '  return data.createBookmark.bookmark;',
    tests: 'src/lib/mutationInvalidation.test.ts',
  },
  {
    name: 'setTopicStatus quên afterMutation() (đánh dấu Xong không lên UI)',
    file: 'src/roadmap/api.ts',
    from: '  assertPayloadOk(data.setTopicStatus);\n  await afterMutation();',
    to: '  assertPayloadOk(data.setTopicStatus);',
    tests: 'src/lib/mutationInvalidation.test.ts',
  },
  {
    name: 'khung bản đồ trả maxX/maxY thô (cắt mất nhãn node cuối)',
    file: 'src/roadmap/map/geometry.ts',
    from: '    maxX: maxX + padding,\n    maxY: maxY + padding,',
    to: '    maxX,\n    maxY,',
    tests: 'src/roadmap/map/geometry.test.ts',
  },
  {
    name: 'nền terrain dùng hạt giống ngẫu nhiên (nhấp nháy mỗi render)',
    file: 'src/roadmap/map/terrain.ts',
    from: '  const rnd = seeded(seed * 7919 + 17);',
    to: '  const rnd = seeded(Date.now());',
    tests: 'src/roadmap/map/terrain.test.ts',
  },
  {
    name: 'cuộn bản đồ không khoá ở 0 (kéo quá đầu trang nhảy vị trí)',
    file: 'src/roadmap/map/MapCanvas.vue',
    from: '  const clamped = Math.max(0, next);',
    to: '  const clamped = next;',
    tests: 'src/roadmap/map/MapCanvas.test.ts',
  },
  {
    name: 'path seed có nút Xoá (server từ chối ⇒ nút chết)',
    file: 'src/routes/RoadmapHome.vue',
    from: '            <Button\n              v-if="!r.path.isBuiltin"',
    to: '            <Button\n              v-if="true"',
    tests: 'src/routes/RoadmapHome.test.ts',
  },
  {
    name: 'vòng tiến độ tự chia lại thay vì đọc summary.percent',
    file: 'src/roadmap/map/ProgressRing.vue',
    from: 'const clamped = computed(() => Math.max(0, Math.min(100, props.percent)));',
    to: 'const clamped = computed(() => 0);',
    tests: 'src/routes/RoadmapHome.test.ts',
  },
];

let red = 0;
const missed = [];

for (const c of cases) {
  const original = readFileSync(c.file, 'utf8');
  if (original.split(c.from).length - 1 !== 1) {
    console.log(`  ⊘ ${c.name} — neo '${c.from.slice(0, 40)}' không xuất hiện đúng 1 lần`);
    missed.push(c.name);
    continue;
  }
  const backup = `/tmp/m6b-${c.tests.replace(/[^a-z]/gi, '-')}.bak`;
  copyFileSync(c.file, backup);
  writeFileSync(c.file, original.replace(c.from, c.to));
  let ok = true;
  try {
    execSync(`pnpm exec vitest run ${c.tests}`, { stdio: 'pipe' });
  } catch {
    ok = false;
  } finally {
    copyFileSync(backup, c.file);
  }
  if (ok) {
    console.log(`  ✗ ${c.name} — vẫn XANH (test không bảo vệ)`);
    missed.push(c.name);
  } else {
    console.log(`  ✓ ${c.name} — ĐỎ như mong đợi`);
    red += 1;
  }
}

console.log(`\n== ${red}/${cases.length} mutation đỏ đúng ==`);
if (missed.length) {
  console.log('Không bảo vệ:', missed.join(' | '));
  process.exitCode = 1;
}
