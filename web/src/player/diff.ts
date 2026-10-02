// T4.2 — word diff transcript STT vs câu mẫu (thuần, không DOM).
// LCS trên token thường (lowercase) để highlight từ sai/thừa/thiếu.
// E1: chuẩn hóa phồn→giản trước khi diff (mirror bảng trong api/simplify.go)
// vì whisper trả phồn còn deck mẫu dùng giản — không convert sẽ chấm sai oan.

export type WordStatus = 'ok' | 'wrong' | 'missing' | 'extra';

export interface DiffToken {
  text: string;
  status: WordStatus;
}

/** Bảng phồn→giản tối thiểu, đồng bộ với tradToSimp (api/simplify.go). */
const tradToSimp: Record<string, string> = {
  '國': '国', '語': '语', '學': '学', '習': '习', '漢': '汉', '麼': '么',
  '嗎': '吗', '們': '们', '說': '说', '話': '话', '愛': '爱', '樂': '乐',
  '龍': '龙', '鳳': '凤', '體': '体', '點': '点', '麵': '面', '頭': '头',
  '會': '会', '過': '过', '還': '还', '這': '这', '個': '个', '來': '来',
  '時': '时', '間': '间', '門': '门', '問': '问', '聽': '听', '讀': '读',
  '寫': '写', '開': '开', '關': '关', '車': '车', '馬': '马', '魚': '鱼',
  '鳥': '鸟', '麥': '麦', '黃': '黄', '傳': '传', '統': '统', '設': '设',
  '備': '备', '術': '术', '雜': '杂', '難': '难', '讓': '让', '認': '认',
  '識': '识', '覺': '觉', '親': '亲', '兒': '儿', '臺': '台', '灣': '湾',
  '廣': '广', '東': '东', '廠': '厂', '遠': '远', '運': '运', '連': '连',
  '適': '适', '選': '选', '錢': '钱', '銀': '银', '辦': '办', '發': '发',
  '長': '长', '對': '对', '導': '导', '後': '后', '復': '复', '複': '复',
  '團': '团', '圖': '图', '價': '价', '優': '优', '單': '单', '標': '标',
  '準': '准', '樣': '样', '檢': '检', '驗': '验', '豐': '丰', '豔': '艳',
  '勵': '励', '慶': '庆',
};

/** Đổi Traditional → Simplified; ký tự ngoài bảng giữ nguyên. */
export function toSimplified(s: string): string {
  return s.replace(/[國語學習漢麼嗎們說話愛樂龍鳳體點麵頭會過還這個來時間門問聽讀寫開關車馬魚鳥麥黃傳統設備術雜難讓認識覺親兒臺灣廣東廠遠運連適選錢銀辦發長對導後復複團圖價優單標準樣檢驗豐豔勵慶]/g, (ch) => tradToSimp[ch] ?? ch);
}

export function tokenize(s: string): string[] {
  return s.split(/\s+/).map((t) => t.trim()).filter(Boolean);
}

function norm(t: string): string {
  return t.toLowerCase().replace(/^[.,!?;:"'()“”‘’—–-]+|[.,!?;:"'()“”‘’—–-]+$/g, '');
}

/** Diff LCS: expected[i] khớp got[j] (so không phân biệt hoa/thường + dấu câu + phồn/giản). */
export function wordDiff(expected: string, got: string): DiffToken[] {
  const e = tokenize(toSimplified(expected));
  const g = tokenize(toSimplified(got));
  const en = e.map(norm);
  const gn = g.map(norm);
  const m = e.length;
  const n = g.length;
  const dp: number[][] = Array.from({ length: m + 1 }, () => new Array(n + 1).fill(0));
  for (let i = m - 1; i >= 0; i--) {
    for (let j = n - 1; j >= 0; j--) {
      dp[i][j] = en[i] === gn[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
    }
  }
  const out: DiffToken[] = [];
  let i = 0;
  let j = 0;
  while (i < m && j < n) {
    if (en[i] === gn[j]) {
      out.push({ text: e[i], status: 'ok' });
      i++;
      j++;
    } else if (dp[i + 1][j] >= dp[i][j + 1]) {
      out.push({ text: e[i], status: 'missing' });
      i++;
    } else {
      out.push({ text: g[j], status: 'extra' });
      j++;
    }
  }
  while (i < m) {
    out.push({ text: e[i], status: 'missing' });
    i++;
  }
  while (j < n) {
    out.push({ text: g[j], status: 'extra' });
    j++;
  }
  // Gộp cặp missing+extra liền kề thành wrong (đọc sai từ đó).
  const merged: DiffToken[] = [];
  for (let k = 0; k < out.length; k++) {
    const cur = out[k];
    const nxt = out[k + 1];
    if (cur.status === 'missing' && nxt && nxt.status === 'extra') {
      merged.push({ text: `${cur.text}→${nxt.text}`, status: 'wrong' });
      k++;
    } else {
      merged.push(cur);
    }
  }
  return merged;
}

/** Các từ sai (wrong + missing) để lưu sổ lỗi và bấm nghe lại TTS. */
export function wrongWords(diff: DiffToken[]): string[] {
  return diff.filter((t) => t.status === 'wrong' || t.status === 'missing').map((t) => t.text);
}

/** Tỉ lệ đúng = token ok / tổng token mẫu (0 khi mẫu rỗng). */
export function diffScore(expected: string, diff: DiffToken[]): number {
  const total = tokenize(expected).length;
  if (total === 0) return 0;
  return diff.filter((t) => t.status === 'ok').length / total;
}
