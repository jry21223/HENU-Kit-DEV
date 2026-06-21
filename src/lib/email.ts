export function generateVerificationCode() {
  return Math.floor(100000 + Math.random() * 900000).toString();
}

export async function sendVerificationEmail(email: string, code: string) {
  if (process.env.EMAIL_PROVIDER === "mock" || process.env.NODE_ENV !== "production") {
    console.log(`[mock-email] verification code for ${email}: ${code}`);
    return;
  }

  throw new Error("Real email provider is not configured yet.");
}

