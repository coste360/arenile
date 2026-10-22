## 4. Struttura del repository

```
arenile/
├── cmd/
│   └── arenile/              # main: comandi serve, migrate, admin, seed, backup
├── internal/
│   ├── app/                  # composizione, registry delle estensioni
│   ├── config/
│   ├── http/
│   │   ├── admin/            # handler API pannello
│   │   ├── public/           # handler API booking
│   │   ├── middleware/
│   │   └── sse/
│   ├── domain/
│   │   ├── establishment/
│   │   ├── mapping/          # mappe, zone, postazioni
│   │   ├── catalog/          # pack, extra, componenti
│   │   ├── availability/
│   │   ├── pricing/
│   │   ├── policy/
│   │   ├── booking/
│   │   ├── customer/
│   │   ├── payment/
│   │   ├── cash/             # cassa e chiusure
│   │   ├── fiscal/
│   │   ├── notification/
│   │   ├── event/            # outbox e webhook
│   │   └── auth/
│   ├── store/                # repository Bun
│   ├── jobs/                 # job runner in-process
│   └── migrations/
│       ├── sqlite/
│       └── postgres/
├── pkg/
│   └── ext/                  # interfacce pubbliche per le estensioni
├── api/
│   └── openapi.yaml
├── web/
│   ├── admin/
│   ├── booking/
│   └── shared/
├── deploy/
│   ├── Dockerfile
│   ├── docker-compose.yml
│   ├── docker-compose.postgres.yml
│   └── coolify/ caprover/
├── docs/                     # architettura, invarianti, ADR, specifica (sezione 27)
├── e2e/                      # Playwright
├── scripts/agents/           # script usati dagli hook degli agenti
├── .claude/                  # regole, skill, subagent, impostazioni (sezione 27)
├── .devcontainer/
├── .github/                  # CI, template di issue e pull request
├── AGENTS.md                 # istruzioni per gli agenti di coding
├── CLAUDE.md                 # importa AGENTS.md
├── Makefile                  # unica interfaccia per build, test e verifiche
├── LICENSE                   # BSL 1.1 (BUSL-1.1), diventa AGPL-3.0 dopo 3 anni
├── CLA.md
├── CHANGELOG.md
└── CONTRIBUTING.md
```

Le interfacce in `pkg/ext` sono le **uniche API Go stabili** per chi estende il core. Tutto ciò che sta in `internal/` può cambiare senza preavviso.

---

