# Privacy Policy (DRAFT — not in force, requires legal review)

> **Draft.** This plain-language draft has not been reviewed by a lawyer and is
> not legal advice. Replace the contact address and have it reviewed before any
> public launch.

## What we store

- An email address and a bcrypt password hash, to operate your account.
- Your handle, and any display name and bio you choose to provide.
- Public wallet addresses you connect (never private keys or seed phrases).
- Records of transactions you submit through OpenHandle: network, signature,
  sender/recipient addresses, integer amount, asset, status and timestamps.
- Verified social account references (provider + stable user id + username) only
  after you complete verification. Access tokens are not retained.
- Security logs and audit events (event type, actor, target, timestamp). We do
  not log passwords, tokens, OAuth codes or seeds.

## What becomes public

If your profile is public: your handle, display name, bio, connected public
wallet addresses, and verified social accounts are visible to anyone, including
through the public resolution API.

**Public wallet addresses and on-chain transactions are inherently public on the
blockchain and are outside OpenHandle's control.** We cannot delete on-chain
records.

## What we never do

- We never collect seed phrases or private keys.
- We never scrape private profile data or access unofficial APIs.
- We never show a verification badge without a real provider verification.
- We never sell your data.

## Your controls

- Make your profile private (it then resolves to "not found").
- Disconnect wallets and social accounts.
- Delete your account: this scrubs your account data and releases your handle.
  On-chain records remain public and permanent.

## Service providers

The backend contacts a Solana RPC endpoint (configurable) to read transaction
status. If social accounts are configured, the backend contacts that provider's
official API. Each provider has its own privacy terms.

## Contact

Privacy questions: `privacy@example.invalid` (placeholder — replace before launch).
