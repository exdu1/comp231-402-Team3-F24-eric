# LitePod

A lightweight container orchestration platform designed for small and medium businesses.

## Setup

### Prerequisites

- Go 1.22 or higher
- containerd (minimum version 1.7.x)
- Linux environment (WSL2 supported)
- Root/sudo access for containerd socket

### Installation

1. **Install containerd**:
```bash
# Install required packages
sudo apt-get update
sudo apt-get install containerd

# Start containerd service
sudo systemctl restart containerd
sudo systemctl enable containerd
```

2. **Install Brew and Go**
```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)" 
```
```bash
test -d ~/.linuxbrew && eval "$(~/.linuxbrew/bin/brew shellenv)"
test -d /home/linuxbrew/.linuxbrew && eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"
echo "eval \"\$($(brew --prefix)/bin/brew shellenv)\"" >> ~/.bashrc
```
```bash
brew install go
```

3. **Clone the repository**:
Make sure to use classic GitHub token: https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens#creating-a-personal-access-token-classic

```bash
brew install gh
gh auth login --with-token < "token_here"
gh repo clone Mindful-Developer/comp231-402-Team3-F24
cd comp231-402-Team3-F24
```

4. **Install dependencies**:
```bash
go mod download
```

### Configuration

1. **Set up containerd permissions**:
```bash
# Create litepod group
sudo groupadd litepod

# Add your user to the litepod group
sudo usermod -aG litepod $USER

# Set containerd socket permissions
sudo chown root:litepod /run/containerd/containerd.sock
sudo chmod 660 /run/containerd/containerd.sock
```

### API Documentation (Swagger)

The API documentation is available through Swagger UI. To access it:

1. **Start the server**:
```bash
go run cmd/litepod/main.go
```

2. **Access Swagger UI**:
Open your web browser and navigate to:
```
http://localhost:8080/swagger/index.html
```

This will provide you with interactive API documentation where you can:
- View all available endpoints
- Test API calls directly from the browser
- See request/response schemas
- Download the OpenAPI specification

### Running Tests

```bash
# Run all tests
sudo $(which go) test ./... -v

# Run specific test package
sudo $(which go) test ./internal/runtime -v

# Skip integration tests
SKIP_INTEGRATION=1 go test ./...
```

### Verifying Installation

1. **Check containerd status**:
```bash
sudo systemctl status containerd
```

2. **Verify socket permissions**:
```bash
sudo ls -l /run/containerd/containerd.sock
```
