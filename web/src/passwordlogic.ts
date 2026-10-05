// The UI password's rules and the strength indicator (DECISIONS P15-7). The
// rules mirror internal/auth.CheckPolicy; the strength is only a hint.

export const MIN_LENGTH = 8;

export type Problem = 'tooShort' | 'noCapital';

/** What the server would refuse, in the order the form lists the rules. */
export function problems(pw: string): Problem[] {
  const out: Problem[] = [];
  if ([...pw].length < MIN_LENGTH) out.push('tooShort');
  if (!/\p{Lu}/u.test(pw)) out.push('noCapital');
  return out;
}

export type Level = 0 | 1 | 2 | 3 | 4;
export const LEVELS = ['veryWeak', 'weak', 'fair', 'good', 'strong'] as const;

// Passwords and patterns everyone tries first.
const COMMON = ['password', 'wachtwoord', 'passwort', 'motdepasse', 'contrase', 'shelly', 'admin', 'welcome', 'qwerty', 'azerty', '123456', 'abcdef', 'letmein', 'iloveyou'];

/** 0 (very weak) … 4 (strong): length and the kinds of characters, less for common words and repeats. */
export function strength(pw: string): Level {
  const chars = [...pw];
  if (chars.length === 0) return 0;
  let score = 0;
  if (chars.length >= MIN_LENGTH) score++;
  if (chars.length >= 12) score++;
  if (chars.length >= 16) score++;
  const kinds = [/\p{Ll}/u, /\p{Lu}/u, /\p{N}/u, /[^\p{L}\p{N}]/u].filter((re) => re.test(pw)).length;
  if (kinds >= 3) score++;
  if (kinds === 4) score++;
  const lower = pw.toLowerCase();
  const unique = new Set(chars).size;
  if (COMMON.some((w) => lower.includes(w)) || unique <= Math.max(2, chars.length / 3)) score = Math.min(score, 1);
  if (problems(pw).length) score = Math.min(score, 1);
  return Math.min(score, 4) as Level;
}
