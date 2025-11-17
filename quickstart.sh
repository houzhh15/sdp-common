#!/usr/bin/env bash

# Quick Start Script for SDP-Common
# This script helps you get started with sdp-common quickly

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print functions
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check prerequisites
check_prerequisites() {
    print_info "Checking prerequisites..."
    
    # Check Go
    if ! command -v go &> /dev/null; then
        print_error "Go is not installed. Please install Go 1.21 or higher."
        echo "Visit: https://go.dev/dl/"
        exit 1
    fi
    
    GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
    print_success "Go version: $GO_VERSION"
    
    # Check Make (optional)
    if command -v make &> /dev/null; then
        print_success "Make is available"
    else
        print_warning "Make is not installed (optional, but recommended)"
    fi
    
    # Check OpenSSL for certificate generation
    if command -v openssl &> /dev/null; then
        print_success "OpenSSL is available"
    else
        print_warning "OpenSSL is not installed (needed for certificate generation)"
    fi
}

# Install dependencies
install_deps() {
    print_info "Installing dependencies..."
    go mod download
    go mod verify
    print_success "Dependencies installed"
}

# Generate test certificates
generate_certs() {
    print_info "Generating test certificates..."
    
    if [ -f "./scripts/generate-certs.sh" ]; then
        chmod +x ./scripts/generate-certs.sh
        ./scripts/generate-certs.sh
        print_success "Test certificates generated in certs/"
    else
        print_error "Certificate generation script not found"
        exit 1
    fi
}

# Run tests
run_tests() {
    print_info "Running tests..."
    go test ./... -v -cover
    print_success "All tests passed"
}

# Build examples
build_examples() {
    print_info "Building example programs..."
    
    mkdir -p bin
    
    # Build Controller
    print_info "Building Controller example..."
    cd examples/controller && go build -o ../../bin/controller-example . && cd ../..
    
    # Build IH Client
    print_info "Building IH Client example..."
    cd examples/ih-client && go build -o ../../bin/ih-client-example . && cd ../..
    
    # Build AH Agent
    print_info "Building AH Agent example..."
    cd examples/ah-agent && go build -o ../../bin/ah-agent-example . && cd ../..
    
    print_success "Example programs built in bin/"
}

# Main menu
show_menu() {
    echo ""
    echo "======================================"
    echo "   SDP-Common Quick Start"
    echo "======================================"
    echo ""
    echo "Select an option:"
    echo "  1) Full Setup (recommended for first time)"
    echo "  2) Install dependencies only"
    echo "  3) Generate test certificates"
    echo "  4) Run tests"
    echo "  5) Build example programs"
    echo "  6) Run Controller example"
    echo "  7) Run IH Client example"
    echo "  8) Run AH Agent example"
    echo "  9) Exit"
    echo ""
    read -p "Enter your choice [1-9]: " choice
    
    case $choice in
        1)
            check_prerequisites
            install_deps
            generate_certs
            run_tests
            build_examples
            print_success "Setup complete!"
            print_info "You can now run example programs from the menu"
            show_menu
            ;;
        2)
            install_deps
            show_menu
            ;;
        3)
            generate_certs
            show_menu
            ;;
        4)
            run_tests
            show_menu
            ;;
        5)
            build_examples
            show_menu
            ;;
        6)
            if [ ! -f "bin/controller-example" ]; then
                print_warning "Controller example not built. Building now..."
                build_examples
            fi
            print_info "Starting Controller example..."
            ./bin/controller-example
            ;;
        7)
            if [ ! -f "bin/ih-client-example" ]; then
                print_warning "IH Client example not built. Building now..."
                build_examples
            fi
            print_info "Starting IH Client example..."
            ./bin/ih-client-example
            ;;
        8)
            if [ ! -f "bin/ah-agent-example" ]; then
                print_warning "AH Agent example not built. Building now..."
                build_examples
            fi
            print_info "Starting AH Agent example..."
            ./bin/ah-agent-example
            ;;
        9)
            print_info "Goodbye!"
            exit 0
            ;;
        *)
            print_error "Invalid option. Please try again."
            show_menu
            ;;
    esac
}

# Main
main() {
    # Check if we're in the right directory
    if [ ! -f "go.mod" ]; then
        print_error "This script must be run from the project root directory"
        exit 1
    fi
    
    print_info "Welcome to SDP-Common Quick Start!"
    
    # If arguments provided, run specific command
    if [ $# -gt 0 ]; then
        case $1 in
            setup)
                check_prerequisites
                install_deps
                generate_certs
                run_tests
                build_examples
                print_success "Setup complete!"
                ;;
            deps)
                install_deps
                ;;
            certs)
                generate_certs
                ;;
            test)
                run_tests
                ;;
            build)
                build_examples
                ;;
            *)
                print_error "Unknown command: $1"
                echo "Usage: $0 [setup|deps|certs|test|build]"
                exit 1
                ;;
        esac
    else
        # Show interactive menu
        show_menu
    fi
}

main "$@"
