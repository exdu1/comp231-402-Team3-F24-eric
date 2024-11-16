# comp231-402-Team3-F24

## Project Structure:
```bash
litepod/
├── cmd/
│   └── litepod/
│       └── main.go           # Application entry point
├── internal/
│   ├── api/                  # API handlers
│   │   ├── handlers.go
│   │   ├── middleware.go
│   │   └── routes.go
│   ├── container/            # Container management
│   │   ├── container.go      # Container operations
│   │   ├── health.go         # Health checking
│   │   └── resource.go       # Resource monitoring
│   ├── pod/                  # Pod management
│   │   ├── pod.go            # Pod operations
│   │   └── validator.go      # Pod validation
│   ├── runtime/              # Container runtime
│   │   ├── runtime.go        # Runtime interface
│   │   └── containerd.go     # ContainerD implementation
│   └── logger/               # Logging package
│       └── logger.go
├── pkg/                      # Public packages
│   ├── types/                # Shared types
│   │   ├── container.go
│   │   └── pod.go
│   └── metrics/              # Metrics collection
│       └── metrics.go
├── web/                      # Frontend assets
│   ├── templates/
│   └── static/
├── configs/                  # Configuration files
├── scripts/                  # Build and deployment scripts
├── test/                     # Integration tests
├── go.mod
├── README.md
├── LICENSE
└── .gitignore
```