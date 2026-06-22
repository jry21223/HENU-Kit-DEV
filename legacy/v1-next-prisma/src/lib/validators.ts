export const HENU_EMAIL_DOMAINS = ["henu.edu.cn", "stu.henu.edu.cn"] as const;

export function normalizeEmail(email: string) {
  return email.trim().toLowerCase();
}

export function isValidEmail(email: string) {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email);
}

export function getEmailDomain(email: string) {
  const normalizedEmail = normalizeEmail(email);
  const atIndex = normalizedEmail.lastIndexOf("@");
  return atIndex >= 0 ? normalizedEmail.slice(atIndex + 1) : "";
}

export function isAllowedStudentEmail(email: string, allowedDomains = HENU_EMAIL_DOMAINS) {
  if (!isValidEmail(email)) {
    return false;
  }
  const domain = getEmailDomain(email);
  return allowedDomains.includes(domain as (typeof allowedDomains)[number]);
}

export function isValidGrade(grade: string) {
  return /^20\d{2}级$/.test(grade);
}

