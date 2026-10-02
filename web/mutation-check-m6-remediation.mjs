// Mutation-check M6 remediation — sửa ngược từng quyết định của lane này,
// test PHẢI ĐỎ, rồi KHÔI PHỤC.
//
// Cách làm y hệt `mutation-check-m6b.mjs`: copy file ra `.bak` TRƯỚC, sửa,
// chạy test, copy lại — không thay chuỗi tại chỗ (bản shell đầu tiên của M6b
// đã hỏng file vì chuỗi rỗng được chèn vào mọi ký tự).
//
// Mỗi mutation ở đây ứng với 1 finding đã được Oracle M6 chấp nhận:
//   F2 `/review?card=` là link chết  ·  F3 `load()` ném về chặng đầu
//   F4 đổi chặng không reset scroll   ·  #5 xoá không có bước xác nhận
//
// Nếu 1 mutation nào vẫn XANH ⇒ test đó bảo vệ sai chỗ (bẫy §8.1 của M6b:
// assert TEXT thay vì assert HÀNH VI), phải sửa test chứ không được bỏ
// mutation.
import { copyFileSync, readFileSync, writeFileSync } from 'node:fs';
import { execSync } from 'node:child_process';

const cases = [
  {
    name: 'F2 · Review.vue bỏ đọc query.card (link /review?card= thành link chết)',
    file: 'src/routes/Review.vue',
    from: 'watch(cardFromQuery, (v) => void loadFocus(v), { immediate: true });',
    to: '// mutation: bỏ đọc query.card',
    tests: 'src/routes/Review.test.ts src/routes/RestoredFeatures.test.ts',
  },
  {
    name: 'F2 · bỏ việc chọn deck của thẻ ?card= (màn vẫn trắng "Chọc deck")',
    file: 'src/routes/Review.vue',
    from: '      deckId.value = card.deckId;',
    to: '      deckId.value = deckFromQuery.value;',
    tests: 'src/routes/Review.test.ts',
  },
  {
    name: 'F2 · thẻ ?card= không lên đầu hàng đợi',
    file: 'src/routes/Review.vue',
    from: '  return [f, ...due.filter((c) => c.id !== f.id)];',
    to: '  return due; // mutation: bỏ đặt thẻ lên đầu',
    tests: 'src/routes/Review.test.ts',
  },
  {
    name: 'F3 · load() tính lại stageId từ localStorage (nhảy về chặng 1)',
    file: 'src/routes/RoadmapMap.vue',
    from: "      (p.stages.some((s) => s.id === stageId.value) ? stageId.value : '') ||",
    to: "      '' ||",
    tests: 'src/routes/RoadmapMap.test.ts',
  },
  {
    name: 'F4 · MapCanvas chỉ watch terrain (2 chặng cùng terrain không reset offset)',
    file: 'src/roadmap/map/MapCanvas.vue',
    from: '  () => `${props.slug}|${props.direction}|${props.nodes.map((n) => n.id).join(\',\')}`,',
    to: '  () => props.terrain,',
    tests: 'src/roadmap/map/MapCanvas.test.ts src/routes/RoadmapMap.test.ts',
  },
  {
    name: 'F4 · MapCanvas bám tham chiếu mảng nodes (cuộn về 0 sau mỗi mutation)',
    file: 'src/roadmap/map/MapCanvas.vue',
    from: '  () => `${props.slug}|${props.direction}|${props.nodes.map((n) => n.id).join(\',\')}`,',
    to: '  () => [props.slug, props.direction, props.nodes],',
    tests: 'src/roadmap/map/MapCanvas.test.ts',
  },
  {
    name: '#5 · xoá chặng gọi thẳng mutation (không bước xác nhận)',
    file: 'src/routes/RoadmapMap.vue',
    from: '            :data-testid="`ask-delete-stage-${stage.id}`"\n            @click="askDelete(\'stage\', stage.id)"',
    to: '            data-testid="ask-delete-stage-x"\n            @click="removeStage(stage)"',
    tests: 'src/routes/RoadmapMap.test.ts',
  },
  {
    name: '#5 · xoá màn gọi thẳng mutation (không bước xác nhận)',
    file: 'src/routes/RoadmapMap.vue',
    from: '                  :data-testid="`ask-delete-topic-${t.id}`"\n                  @click="askDelete(\'topic\', t.id)"',
    to: '                  data-testid="ask-delete-topic-x"\n                  @click="removeTopic(t)"',
    tests: 'src/routes/RoadmapMap.test.ts',
  },
  {
    name: '#5 · xoá mốc chặng gọi thẳng mutation (không bước xác nhận)',
    file: 'src/routes/RoadmapMap.vue',
    from: '                    :data-testid="`ask-delete-milestone-${m.id}`"\n                    @click="askDelete(\'milestone\', m.id)"',
    to: '                    data-testid="ask-delete-milestone-x"\n                    @click="removeMilestone(m)"',
    tests: 'src/routes/RoadmapMap.test.ts',
  },
  {
    name: 'F2 · ErrorBook bỏ ?card= khỏi link (link trỏ về /review trần)',
    file: 'src/routes/ErrorBook.vue',
    from: ':to="`/review?card=${t.cardId}`"',
    to: ':to="\'/review\'"',
    tests: 'src/routes/RestoredFeatures.test.ts',
  },
  {
    name: 'F2 · harness stub RouterLink thành <a> rỗng (bẫy #1 §8.1 quay lại)',
    file: 'src/test/harness.ts',
    from: "stubs: { RouterLink: { props: ['to'], template: '<a :href=\"to\"><slot /></a>' } },",
    to: "stubs: { RouterLink: { template: '<a><slot /></a>' } },",
    tests: 'src/routes/RestoredFeatures.test.ts',
  },
];

let red = 0;
const missed = [];

for (const c of cases) {
  const original = readFileSync(c.file, 'utf8');
  if (original.split(c.from).length - 1 !== 1) {
    console.log(`  ⊘ ${c.name} — neo không xuất hiện đúng 1 lần`);
    missed.push(c.name);
    continue;
  }
  const backup = `/tmp/m6r-${c.file.replace(/[^a-z]/gi, '-')}.bak`;
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
