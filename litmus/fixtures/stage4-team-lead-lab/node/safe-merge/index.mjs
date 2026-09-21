export function safeMerge(base, patch) {
  return { ...base, ...patch };
}
