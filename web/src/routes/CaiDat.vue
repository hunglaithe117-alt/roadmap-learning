<script setup lang="ts">
// M5 — port từ `routes/CaiDat.tsx`.
//
// 3 điểm khác bản v1, đều là hệ quả của app-v2 và PHẢI hiện rõ cho người dùng:
//
//  1. **Backup / Restore trả 501** (`BackupPort` chưa implement — Postgres không
//     có `VACUUM INTO`, cần `pg_dump`/`pg_restore` thuộc M7). Bản v1 có `<a
//     download>` tới `/api/backup`: bấm là trình duyệt tải về 1 file JSON lỗi
//     rồi tưởng là backup. Ở đây gọi thật qua vue-query và hiện message tiếng
//     Việt của server — không bao giờ trông như thành công.
//  2. **Đồng bộ peer không nhận file nữa**: app-v2 chưa nối peer (M4 §8), nên
//     `sync` là mutation không tham số. Nút vẫn còn, gọi để hiện lỗi nguyên văn
//     thay vì giấu.
//  3. Ô "API base" đọc/ghi qua `lib/apiBase` — đổi base phải `resetUrqlClient()`
//     để client urql dựng lại URL, không thì vẫn gọi host cũ.
import { onMounted, ref } from 'vue';
import NoticeBar from '../components/NoticeBar.vue';
import { resetUrqlClient } from '../graphql/client';
import { errorMessage } from '../graphql/errors';
import { getApiBase, setApiBase } from '../lib/apiBase';
import {
  buildTTSUrl,
  uploadSTT,
  type STTResult,
} from '../rest/client';
import { useBackupQuery, useHealthQuery, useRestoreMutation, useTtsEngineQuery } from '../rest/useRest';
import { readConfettiEnabled, writeConfettiEnabled } from '../roadmap/map/viewPrefs';
import {
  fetchSyncConflicts,
  fetchSyncStatus,
  isSyncUiEnabled,
  runSync,
  setSyncUiEnabled,
  type SyncConflict,
} from '../player/api';

const apiBase = ref(getApiBase());
const healthOn = ref(false);
const ttsText = ref('ni hao');
const ttsUrl = ref('');
const recState = ref<'idle' | 'recording' | 'uploading'>('idle');
const transcript = ref('');
const audioErr = ref('');
const syncOn = ref(isSyncUiEnabled());
const syncInfo = ref('');
const syncMsg = ref('');
const syncWarnings = ref<string[]>([]);
const syncConflictsNow = ref<SyncConflict[]>([]);
const conflicts = ref<SyncConflict[]>([]);
const lastSyncAt = ref('');
const syncing = ref(false);
const restoreMsg = ref('');

// ── API base + health ───────────────────────────────────────────────────────
function saveBase() {
  setApiBase(apiBase.value);
  // URL của client urql đã đóng băng lúc module load ⇒ phải dựng lại, nếu không
  // thì đổi ô này xong mọi request vẫn về host cũ (rất khó đoán).
  resetUrqlClient();
}

const { data: health, error: healthErr, isFetching: healthBusy } = useHealthQuery(healthOn);

// ── TTS ─────────────────────────────────────────────────────────────────────
const engineText = ref('');
const engineOn = ref(false);
const { data: ttsEngine } = useTtsEngineQuery(engineText, engineOn);

function playTTS() {
  ttsUrl.value = buildTTSUrl(ttsText.value);
  // Engine lấy theo text vừa gõ: chuyển sang ref trước rồi mới bật query, nếu
  // không thì lần đầu sẽ hỏi text rỗng.
  engineText.value = ttsText.value;
  engineOn.value = true;
}

// ── Recorder → STT ──────────────────────────────────────────────────────────
let recorder: MediaRecorder | null = null;
let chunks: Blob[] = [];

async function startRecording() {
  audioErr.value = '';
  if (typeof MediaRecorder === 'undefined') {
    audioErr.value = 'MediaRecorder not supported in this browser';
    return;
  }
  try {
    const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
    const rec = new MediaRecorder(stream);
    chunks = [];
    rec.ondataavailable = (e) => {
      if (e.data.size > 0) chunks.push(e.data);
    };
    rec.onstop = () => {
      stream.getTracks().forEach((t) => t.stop());
    };
    recorder = rec;
    rec.start();
    recState.value = 'recording';
  } catch (e) {
    audioErr.value = errorMessage(e);
  }
}

async function stopAndUpload() {
  const rec = recorder;
  if (!rec) return;
  recState.value = 'uploading';
  const done = new Promise<Blob>((resolve) => {
    rec.onstop = () => resolve(new Blob(chunks, { type: rec.mimeType || 'audio/webm' }));
  });
  rec.stop();
  try {
    const r: STTResult = await uploadSTT(await done);
    transcript.value = `${r.transcript} (${r.lang}, ${r.engine}) [X-Engine: ${r.engineHeader ?? r.engine}]`;
    audioErr.value = '';
  } catch (e) {
    audioErr.value = errorMessage(e);
  } finally {
    recState.value = 'idle';
  }
}

// ── Backup / Restore (501 tới M7) ───────────────────────────────────────────
const backupOn = ref(false);
const backup = useBackupQuery(backupOn);
const restore = useRestoreMutation();
// Khi server đã có `pg_dump` (M7) nút tải sẽ tự hiện — hiện tại luôn 501 nên
// nhánh thành công là đường chưa dùng tới, nhưng vẫn phải viết đúng.

function saveBackupBlob() {
  const blob = backup.data.value;
  if (!blob) return;
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = 'langapp.dump';
  a.click();
  URL.revokeObjectURL(url);
}

async function onRestoreFile(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0];
  if (!f) return;
  try {
    const r = await restore.mutateAsync(f);
    restoreMsg.value = `Restore xong (${r.restored ?? '?'} byte). Tải lại trang để thấy dữ liệu mới.`;
  } catch (err) {
    restoreMsg.value = errorMessage(err);
  }
}

// ── Sync ────────────────────────────────────────────────────────────────────
async function refreshSyncLog() {
  try {
    const [c, s] = await Promise.all([fetchSyncConflicts(), fetchSyncStatus()]);
    conflicts.value = c;
    lastSyncAt.value = s.lastSyncAt;
  } catch {
    /* server chưa có bảng sync — giữ log trống, không báo lỗi chết màn */
  }
}

onMounted(refreshSyncLog);

async function doSync() {
  syncing.value = true;
  syncWarnings.value = [];
  syncConflictsNow.value = [];
  try {
    const r = await runSync();
    syncMsg.value = `Đồng bộ xong: +${r.merged.decks} decks, +${r.merged.cards} cards, +${r.merged.reviews} reviews, +${r.merged.notes} notes.`;
    syncWarnings.value = r.warnings;
    syncConflictsNow.value = r.conflicts;
    lastSyncAt.value = r.lastSyncAt;
    await refreshSyncLog();
  } catch (e) {
    syncMsg.value = errorMessage(e);
  } finally {
    syncing.value = false;
  }
}

async function onSyncToggle(e: Event) {
  const on = (e.target as HTMLInputElement).checked;
  syncOn.value = on;
  setSyncUiEnabled(on);
  if (!on) {
    syncInfo.value = 'Đã tắt sync — app chạy offline single-user.';
    return;
  }
  try {
    const s = await fetchSyncStatus();
    syncInfo.value = `Server sync sẵn sàng (${s.strategy}) — lần merge gần nhất: ${s.lastSyncAt || 'chưa có'}.`;
  } catch (err) {
    syncInfo.value = errorMessage(err);
  }
}

function shortGuid(g: string): string {
  return g.slice(0, 8);
}

// ── hiệu ứng bản đồ ────────────────────────────────────────────────────────
const confettiOn = ref(readConfettiEnabled());

function saveConfetti(): void {
  writeConfettiEnabled(confettiOn.value);
}
</script>

<template>
  <div>
    <h2 class="text-xl font-semibold">Cài đặt</h2>

    <section class="mt-4">
      <h3 class="font-semibold">API</h3>
      <div class="mt-2 flex flex-wrap items-center gap-2">
        <input
          v-model="apiBase"
          class="rounded-card border border-line px-2 py-1"
          placeholder="API base (trống = cùng origin)"
          aria-label="api base"
          style="width: 320px"
        />
        <button
          type="button"
          class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
          @click="saveBase"
        >
          Lưu
        </button>
        <button
          type="button"
          class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
          @click="healthOn = true"
        >
          Check /api/health
        </button>
      </div>
      <p v-if="healthBusy" class="text-muted">đang kiểm tra…</p>
      <p v-if="health" data-testid="health">health: {{ health.status }}</p>
      <NoticeBar v-if="healthErr" :text="errorMessage(healthErr)" />
    </section>

    <section class="mt-4">
      <h3 class="font-semibold">TTS playback</h3>
      <div class="mt-2 flex flex-wrap items-center gap-2">
        <input
          v-model="ttsText"
          class="rounded-card border border-line px-2 py-1"
          placeholder="text for TTS"
          aria-label="tts text"
        />
        <button
          type="button"
          class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
          @click="playTTS"
        >
          Play TTS
        </button>
      </div>
      <audio v-if="ttsUrl" controls :src="ttsUrl" />
      <p
        v-if="ttsEngine"
        data-testid="tts-engine-badge"
        title="Engine TTS đang chạy (đọc từ header X-Engine)"
        class="text-muted"
      >
        X-Engine: {{ ttsEngine }}
      </p>
    </section>

    <section class="mt-4">
      <h3 class="font-semibold">Recorder → STT</h3>
      <div class="mt-2">
        <button
          v-if="recState === 'recording'"
          type="button"
          class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
          @click="stopAndUpload"
        >
          Stop &amp; transcribe
        </button>
        <button
          v-else
          type="button"
          class="rounded-card border border-line px-3 py-1 hover:border-line-strong disabled:opacity-50"
          :disabled="recState === 'uploading'"
          @click="startRecording"
        >
          {{ recState === 'uploading' ? 'Uploading…' : 'Record' }}
        </button>
      </div>
      <p v-if="transcript">transcript: {{ transcript }}</p>
    </section>
    <NoticeBar v-if="audioErr" :text="audioErr" />

    <section class="mt-4">
      <h3 class="font-semibold">Backup / Restore (Postgres dump — GHI ĐÈ hủy diệt)</h3>
      <p class="mt-1">
        <strong>Restore ghi đè toàn bộ dữ liệu máy này bằng file backup.</strong>
        Muốn giữ cả hai máy, dùng Đồng bộ (merge) ở dưới thay vì restore.
      </p>
      <p class="mt-1 text-warn">
        Tính năng backup/restore <strong>chưa dùng được</strong> ở bản này: Postgres
        không có <code>VACUUM INTO</code>, cần <code>pg_dump</code>/<code
          >pg_restore</code
        >. Server trả <code>501</code> — bấm nút sẽ thấy message của server thay vì
        tải về 1 file hỏng.
      </p>
      <div class="mt-2 flex flex-wrap items-center gap-2">
        <button
          type="button"
          class="rounded-card border border-line px-3 py-1 hover:border-line-strong"
          @click="backupOn = true"
        >
          Kiểm tra &amp; tải backup
        </button>
      </div>
      <NoticeBar v-if="backup.error.value" :text="errorMessage(backup.error.value)" />
      <NoticeBar v-if="backup.isSuccess.value" tone="ok" text="Đã tải được backup." />
      <button
        v-if="backup.data.value"
        type="button"
        class="ml-2 rounded-card border border-line px-3 py-1 hover:border-line-strong"
        @click="saveBackupBlob"
      >
        Lưu file backup (.dump)
      </button>
      <input
        type="file"
        class="mt-2 block"
        accept=".db,.dump,.sqlite,.sqlite3,application/x-sqlite3"
        aria-label="file backup để restore"
        @change="onRestoreFile"
      />
      <NoticeBar :text="restoreMsg" />
    </section>

    <section class="mt-4">
      <h3 class="font-semibold">Đồng bộ peer (merge giữ cả hai — KHÔNG ghi đè)</h3>
      <p class="mt-1">
        Bản này <strong>chưa nối peer</strong>: app-v2 không nhận file backup qua HTTP,
        nên không có ô chọn file. Nút bên dưới gọi thẳng merge trên server; lỗi sẽ
        hiện nguyên văn thay vì im lặng.
      </p>
      <p class="mt-1">
        Đồng bộ là <strong>1 chiều</strong> (máy kia → máy này): muốn 2 chiều thì làm
        2 lượt ngược nhau.
      </p>
      <button
        type="button"
        class="mt-2 rounded-card border border-line px-3 py-1 hover:border-line-strong disabled:opacity-50"
        :disabled="syncing"
        @click="doSync"
      >
        {{ syncing ? 'Đang đồng bộ…' : 'Đồng bộ ngay' }}
      </button>
      <NoticeBar v-if="syncMsg" tone="info" :text="syncMsg" />
      <ul v-if="syncWarnings.length > 0">
        <li v-for="(wn, i) in syncWarnings" :key="i" class="text-warn">{{ wn }}</li>
      </ul>
      <div v-if="syncConflictsNow.length > 0">
        <h4>Xung đột trong lần merge này ({{ syncConflictsNow.length }})</h4>
        <table class="mt-1 border-collapse text-sm">
          <thead>
            <tr>
              <th class="border border-line-soft px-2 py-1">table</th>
              <th class="border border-line-soft px-2 py-1">guid</th>
              <th class="border border-line-soft px-2 py-1">winner</th>
              <th class="border border-line-soft px-2 py-1">detail</th>
              <th class="border border-line-soft px-2 py-1">at</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(c, i) in syncConflictsNow" :key="i">
              <td class="border border-line-soft px-2 py-1">{{ c.table }}</td>
              <td class="border border-line-soft px-2 py-1" :title="c.guid">
                {{ shortGuid(c.guid) }}
              </td>
              <td class="border border-line-soft px-2 py-1">{{ c.winner }}</td>
              <td class="border border-line-soft px-2 py-1">{{ c.detail }}</td>
              <td class="border border-line-soft px-2 py-1">{{ c.resolvedAt }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <section class="mt-4">
      <h3 class="font-semibold">Log xung đột + lần đồng bộ gần nhất (chỉ xem)</h3>
      <p class="mt-1">
        last_sync_at: {{ lastSyncAt || '(chưa đồng bộ lần nào)' }}
        <button
          type="button"
          class="ml-2 rounded-card border border-line px-2 py-0.5 hover:border-line-strong"
          @click="refreshSyncLog"
        >
          Tải lại log
        </button>
      </p>
      <p v-if="conflicts.length === 0">Chưa có xung đột nào được ghi nhận.</p>
      <table v-else class="mt-1 border-collapse text-sm">
        <thead>
          <tr>
            <th class="border border-line-soft px-2 py-1">table</th>
            <th class="border border-line-soft px-2 py-1">guid</th>
            <th class="border border-line-soft px-2 py-1">winner</th>
            <th class="border border-line-soft px-2 py-1">detail</th>
            <th class="border border-line-soft px-2 py-1">at</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(c, i) in conflicts" :key="i">
            <td class="border border-line-soft px-2 py-1">{{ c.table }}</td>
            <td class="border border-line-soft px-2 py-1" :title="c.guid">
              {{ shortGuid(c.guid) }}
            </td>
            <td class="border border-line-soft px-2 py-1">{{ c.winner }}</td>
            <td class="border border-line-soft px-2 py-1">{{ c.detail }}</td>
            <td class="border border-line-soft px-2 py-1">{{ c.resolvedAt }}</td>
          </tr>
        </tbody>
      </table>
    </section>

    <section class="mt-4">
      <h3 class="font-semibold">Hiệu ứng bản đồ</h3>
      <p class="mt-1">
        Đánh dấu Xong 1 màn thì bản đồ nổ mảnh giấy. Tắt ở đây thì màn vẫn đánh
        dấu Xong bình thường, chỉ không có hiệu ứng.
      </p>
      <label class="mt-2 inline-flex items-center gap-2">
        <input
          v-model="confettiOn"
          type="checkbox"
          aria-label="bật hiệu ứng confetti"
          @change="saveConfetti"
        />
        Cho phép hiệu ứng confetti
      </label>
      <p class="text-muted">
        Máy đang đặt “giảm chuyển động” thì hiệu ứng vẫn tắt dù cờ này bật.
      </p>
    </section>

    <section class="mt-4">
      <h3 class="font-semibold">Cờ sync thử nghiệm (UI cũ — merge ở trên luôn sẵn sàng)</h3>
      <label class="mt-2 inline-flex items-center gap-2">
        <input
          type="checkbox"
          :checked="syncOn"
          aria-label="bật sync thử nghiệm"
          @change="onSyncToggle"
        />
        Bật sync thử nghiệm
      </label>
      <p v-if="syncInfo" class="text-muted">{{ syncInfo }}</p>
      <p v-if="!syncOn">Sync tắt: học offline 1 máy, chuyển máy bằng backup/restore.</p>
    </section>
  </div>
</template>
