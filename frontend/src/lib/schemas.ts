import { z } from "zod";

export const emailSchema = z.string().trim().toLowerCase().email("Enter a valid email address.");

export const passwordSchema = z
  .string()
  .min(8, "Password must be at least 8 characters.")
  .max(72, "Password must be at most 72 characters.");

export const credentialsSchema = z.object({
  email: emailSchema,
  password: passwordSchema,
});

// Mirrors backend/internal/validate/handle.go exactly.
export const HANDLE_MIN = 3;
export const HANDLE_MAX = 20;
const RESERVED = new Set([
  "admin", "administrator", "support", "security", "api", "help", "root", "system", "sys", "me",
  "you", "settings", "login", "logout", "register", "signup", "signin", "signout", "app", "docs",
  "doc", "about", "pricing", "terms", "privacy", "legal", "blog", "news", "status", "health",
  "healthz", "readyz", "explore", "how-it-works", "howitworks", "developers", "developer", "wallet",
  "wallets", "payment", "payments", "pay", "send", "receive", "identity", "identities", "profile",
  "profiles", "handle", "handles", "openhandle", "www", "mail", "email", "ftp", "cdn", "static",
  "assets", "public", "private", "null", "undefined", "true", "false", "anonymous", "owner", "mod",
  "moderator", "staff", "team", "official", "verify", "verified",
]);

export const handleSchema = z
  .string()
  .trim()
  .toLowerCase()
  .min(HANDLE_MIN, `Handle must be at least ${HANDLE_MIN} characters.`)
  .max(HANDLE_MAX, `Handle must be at most ${HANDLE_MAX} characters.`)
  .regex(/^[a-z0-9](?:[a-z0-9]|_(?!_))*[a-z0-9]$/, "Use lowercase letters, numbers and single underscores; start and end with a letter or number.")
  .refine((h) => !RESERVED.has(h), "This handle is reserved.");

export const bioSchema = z.string().max(280, "Bio must be 280 characters or fewer.");
export const displayNameSchema = z.string().max(60, "Display name must be 60 characters or fewer.");
