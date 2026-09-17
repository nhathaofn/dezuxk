# Gateway Manager

A desktop application for managing an API Gateway, built with [Wails](https://wails.io/).

> **Gateway Manager is currently only an application shell. Gateway management functionality has not been implemented yet.**

## Status

This project is at the **scaffolding stage**. The desktop application shell is functional with navigation and placeholder pages, but no gateway management features have been implemented.

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
│   ├── config/                # Future: application configuration
│   ├── services/              # Future: gateway admin API client
│   └── models/                # Future: domain models
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
