export function yuanToCents(value: unknown) {
  const amount = Number(value);
  if (!Number.isFinite(amount) || amount < 0) {
    return null;
  }

  return Math.round(amount * 100);
}

export function centsToYuanString(value: number) {
  if (!Number.isInteger(value) || value < 0) {
    return "0.00";
  }

  return (value / 100).toFixed(2);
}
