/** Normalize API / network errors for user-facing English messages. */
export function toUserMessage(err: unknown): string {
  if (err instanceof Error && err.message) {
    const m = err.message.trim();
    if (m === "invalid credentials") {
      return "Invalid username or password.";
    }
    if (m === "current password incorrect") {
      return "Current password is incorrect.";
    }
    return m;
  }
  return "Something went wrong. Please try again.";
}
