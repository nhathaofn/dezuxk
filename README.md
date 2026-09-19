# Gateway Manager

A desktop application for managing an API Gateway, built with [Wails](https://wails.io/).

Gateway Manager currently includes local admin authentication, shared Google account session management for Flow and Gemini, isolated Chrome profiles, SQLite persistence, and a local gateway health endpoint.

## Status

The application is intended for a trusted local Windows user. Google sessions are stored in SQLite with an AES-GCM data key protected by Windows DPAPI, and account backups are password-protected archives.

## First-run and configuration

- On a new database, the first screen creates the single local administrator. Use a password of at least 5 characters.
- For unattended first-run provisioning, set `GATEWAY_ADMIN_PASSWORD` before starting the application. The value is used only when no local user exists.
- `GATEWAY_DB_PATH` selects the SQLite database. `GATEWAY_DATA_PATH` selects Chrome profiles and temporary data; when omitted it is resolved next to the database, avoiding multiple relative `gateway.db` locations.
- The gateway binds to `127.0.0.1` by default. Set `GATEWAY_BIND_ADDRESS` only when an explicit LAN-facing deployment is intended.

## Google account data flow

- Interactive login opens a dedicated Chrome profile and reads session cookies through a loopback CDP port. The account is saved only after Google cookies and an account email are identified.
- Manual and bulk import accept an email plus an existing Google cookie header. Google passwords and recovery emails are not accepted or stored.
- New cookies, session tokens, and proxy values are encrypted before SQLite writes. Legacy plaintext session fields are migrated at the next startup; legacy Google password/recovery columns are cleared.
- Account list responses contain capability flags and a credential-free proxy display. The full proxy is requested only when the authenticated user opens its editor.
- Exported backups require a separate password of at least 5 characters. Older compatible external G-Labs backups can still be imported.

## Live Flow and Gemini quota

- One successful Google login is represented by one managed Chrome profile. The same Google session is used for both `flow.google.com` and `gemini.google.com`; a new interactive login is recorded as `flow,gemini`.
- `GetLiveGoogleAccountMetrics` opens that profile headlessly, reads Flow and Gemini, and returns an in-memory snapshot. It does not write quota or credit values to SQLite, and the UI polls it every 30 seconds while the account is active.
- Flow currently exposes a combined remaining credit value in the authenticated page. The response keeps separate daily/monthly fields with explicit availability flags; if Google does not render those buckets, the UI shows `—` instead of estimating them.
- Gemini is read from its first-party usage page and shows the current and weekly remaining percentages plus reset labels. Legacy `google_accounts.credits` remains only for backwards-compatible imports and old data; it is not used by the live quota UI or updated by session refresh.

## Technology Stack

| Layer     | Technology                             |
| --------- | -------------------------------------- |
| Desktop   | [Wails v2](https://wails.io/)         |
| Backend   | [Go](https://go.dev/)                 |
| Frontend  | [React](https://react.dev/)           |
| Language  | [TypeScript](https://typescriptlang.org/) |
| Bundler   | [Vite](https://vite.dev/)             |
| Styling   | [Tailwind CSS v4](https://tailwindcss.com/) |
| UI        | [shadcn/ui](https://ui.shadcn.com/)   |
| Routing   | [React Router](https://reactrouter.com/) |
| Icons     | [Lucide](https://lucide.dev/)         |

## Prerequisites

- [Go](https://go.dev/dl/) (1.21+)
- [Node.js](https://nodejs.org/) (18+)
- [Wails CLI v2](https://wails.io/docs/gettingstarted/installation)

Install Wails CLI:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

## Development

Run in development mode with hot-reload:

```bash
wails dev
```

This starts the Go backend and Vite dev server together. Frontend changes will hot-reload automatically.

## Build

Build the production desktop application:

```bash
wails build
```

The compiled binary will be output to `build/bin/`.

## Project Structure

```
dezuxk/
├── main.go                    # Application entry point
├── app.go                     # App struct with Wails lifecycle methods
├── wails.json                 # Wails configuration
├── go.mod                     # Go module definition
│
├── internal/                  # Go internal packages
│   ├── config/                # Environment and path configuration
│   ├── crypto/                # DPAPI/AES-GCM data and backup encryption
│   ├── db/                    # SQLite schema, migrations, and persistence
│   ├── security/              # Windows ACL hardening
│   ├── services/              # Auth, gateway, Google account, and Chrome flows
│   └── models/                # Domain and Wails response models
│
├── frontend/                  # React frontend
│   ├── index.html
│   ├── package.json
│   ├── vite.config.ts
│   ├── tsconfig.json
│   ├── components.json        # shadcn/ui configuration
│   │
│   └── src/
│       ├── main.tsx           # Frontend entry point
│       ├── index.css          # Global styles & Tailwind config
│       │
│       ├── app/               # Application root component & routing
│       ├── components/
│       │   ├── layout/        # Layout components (Sidebar, Header, etc.)
│       │   └── ui/            # shadcn/ui components
│       │
│       ├── pages/             # Page components
│       ├── hooks/             # Custom React hooks
│       ├── lib/               # Utilities & future API client
│       │   └── api/           # Future: Admin API client
│       ├── types/             # Shared TypeScript types
│       └── assets/            # Static assets
│
└── build/                     # Build output & platform configs
```

## License

This project is not yet licensed.
