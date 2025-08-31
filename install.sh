#!/bin/bash

set -e

REPO="vadiminshakov/storyshort"
BINARY_NAME="storyshort"

# Set install directory based on OS
case "$(uname -s)" in
    Darwin)
        # macOS - create app bundle in user Applications
        INSTALL_DIR="$HOME/Applications"
        APP_BUNDLE="StoryShort.app"
        mkdir -p "$INSTALL_DIR"
        ;;
    *)
        # Linux and others - use local bin
        if [ -d "$HOME/.local/bin" ]; then
            INSTALL_DIR="$HOME/.local/bin"
        elif [ -d "$HOME/bin" ]; then
            INSTALL_DIR="$HOME/bin"
        else
            INSTALL_DIR="$HOME/.local/bin"
            mkdir -p "$INSTALL_DIR"
        fi
        ;;
esac

detect_os_arch() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)
    
    case $OS in
        darwin)
            OS="darwin"
            ;;
        linux)
            OS="linux"
            ;;
        *)
            echo "Unsupported OS: $OS"
            exit 1
            ;;
    esac
    
    case $ARCH in
        x86_64)
            ARCH="amd64"
            ;;
        arm64|aarch64)
            ARCH="arm64"
            ;;
        *)
            echo "Unsupported architecture: $ARCH"
            exit 1
            ;;
    esac
    
    echo "Detected OS: $OS, Architecture: $ARCH"
}

get_latest_release() {
    echo "Getting latest release info..."
    RELEASE_URL="https://api.github.com/repos/$REPO/releases/latest"
    RELEASE_INFO=$(curl -s "$RELEASE_URL")
    
    if echo "$RELEASE_INFO" | grep -q "Not Found"; then
        echo "No releases found for $REPO"
        exit 1
    fi
    
    TAG_NAME=$(echo "$RELEASE_INFO" | grep '"tag_name"' | cut -d'"' -f4)
    echo "Latest release: $TAG_NAME"
}

download_and_install() {
    ASSET_NAME="${BINARY_NAME}_${OS}_${ARCH}"
    if [ "$OS" = "darwin" ]; then
        ASSET_NAME="${ASSET_NAME}.tar.gz"
    else
        ASSET_NAME="${ASSET_NAME}.tar.gz"
    fi
    
    echo "Looking for asset: $ASSET_NAME"
    
    DOWNLOAD_URL=$(echo "$RELEASE_INFO" | grep -o "https://github.com/$REPO/releases/download/[^\"]*$ASSET_NAME")
    
    if [ -z "$DOWNLOAD_URL" ]; then
        echo "Asset not found: $ASSET_NAME"
        echo "Available assets:"
        echo "$RELEASE_INFO" | grep '"browser_download_url"' | cut -d'"' -f4
        exit 1
    fi
    
    echo "Downloading from: $DOWNLOAD_URL"
    
    TMP_DIR=$(mktemp -d)
    cd "$TMP_DIR"
    
    curl -L -o "$ASSET_NAME" "$DOWNLOAD_URL"
    
    if [ "${ASSET_NAME##*.}" = "gz" ]; then
        tar -xzf "$ASSET_NAME"
    else
        echo "Unsupported archive format"
        exit 1
    fi
    
    if [ ! -f "$BINARY_NAME" ]; then
        echo "Binary $BINARY_NAME not found in archive"
        ls -la
        exit 1
    fi
    
    chmod +x "$BINARY_NAME"
    
    if [ "$(uname -s)" = "Darwin" ]; then
        # macOS - create app bundle
        echo "Creating macOS app bundle..."
        APP_PATH="$INSTALL_DIR/$APP_BUNDLE"
        
        mkdir -p "$APP_PATH/Contents/MacOS"
        mkdir -p "$APP_PATH/Contents/Resources"
        
        # Move binary to app bundle
        mv "$BINARY_NAME" "$APP_PATH/Contents/MacOS/"
        
        # Create Info.plist
        cat > "$APP_PATH/Contents/Info.plist" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleExecutable</key>
    <string>$BINARY_NAME</string>
    <key>CFBundleIdentifier</key>
    <string>com.vadiminshakov.storyshort</string>
    <key>CFBundleName</key>
    <string>StoryShort</string>
    <key>CFBundleVersion</key>
    <string>1.0</string>
    <key>CFBundleShortVersionString</key>
    <string>1.0</string>
    <key>CFBundleInfoDictionaryVersion</key>
    <string>6.0</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>LSMinimumSystemVersion</key>
    <string>10.15</string>
</dict>
</plist>
EOF
        
        echo "Installed to $APP_PATH"
        echo "You can now find StoryShort in your Applications folder"
        
    else
        # Linux - install to bin directory
        echo "Installing to $INSTALL_DIR/$BINARY_NAME"
        mv "$BINARY_NAME" "$INSTALL_DIR/"
        
        # Add to PATH if not already there
        if ! echo "$PATH" | grep -q "$INSTALL_DIR"; then
            echo "Adding $INSTALL_DIR to PATH..."
            
            # Detect shell and update appropriate config file
            SHELL_NAME=$(basename "$SHELL")
            case $SHELL_NAME in
                bash)
                    echo "export PATH=\"\$PATH:$INSTALL_DIR\"" >> "$HOME/.bashrc"
                    echo "Added to ~/.bashrc - restart terminal or run: source ~/.bashrc"
                    ;;
                zsh)
                    echo "export PATH=\"\$PATH:$INSTALL_DIR\"" >> "$HOME/.zshrc"
                    echo "Added to ~/.zshrc - restart terminal or run: source ~/.zshrc"
                    ;;
                fish)
                    echo "set -U fish_user_paths $INSTALL_DIR \$fish_user_paths" >> "$HOME/.config/fish/config.fish"
                    echo "Added to fish config - restart terminal"
                    ;;
                *)
                    echo "export PATH=\"\$PATH:$INSTALL_DIR\"" >> "$HOME/.profile"
                    echo "Added to ~/.profile - restart terminal or run: source ~/.profile"
                    ;;
            esac
        fi
        
        echo "Run '$BINARY_NAME' to start the application"
    fi
    
    cd - > /dev/null
    rm -rf "$TMP_DIR"
    
    echo "Installation completed!"
}

check_dependencies() {
    if ! command -v curl &> /dev/null; then
        echo "curl is required but not installed"
        exit 1
    fi
    
    if ! command -v tar &> /dev/null; then
        echo "tar is required but not installed"
        exit 1
    fi
}

main() {
    echo "StoryShort Installer"
    echo "===================="
    
    check_dependencies
    detect_os_arch
    get_latest_release
    download_and_install
}

main "$@"