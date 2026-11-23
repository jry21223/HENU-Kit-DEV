export function matchesSearchText(query: string, ...fields: string[]): boolean {
  const needle = query.trim().toLocaleLowerCase();
  return needle === "" || fields.some((field) => field.toLocaleLowerCase().includes(needle));
}
