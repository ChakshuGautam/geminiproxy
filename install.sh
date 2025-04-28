#!/bin/bash

# Exit on error
set -e

# Check if running as root
if [ "$EUID" -ne 0 ]; then
  echo "Please run as root (use sudo)"
  exit 1
fi

# Define variables
INSTALL_DIR="/opt/geminiproxy"
SERVICE_NAME="geminiproxy"
SERVICE_FILE="${SERVICE_NAME}.service"
BINARY_NAME="geminiproxy"
KEYS_FILE="gemini.keys"

# Create installation directory
echo "Creating installation directory at ${INSTALL_DIR}..."
mkdir -p ${INSTALL_DIR}

# Build the binary
echo "Building geminiproxy binary..."
go build -o ${BINARY_NAME} ./cmd/main.go

# Copy files to installation directory
echo "Copying files to ${INSTALL_DIR}..."
cp ${BINARY_NAME} ${INSTALL_DIR}/
cp ${SERVICE_FILE} /etc/systemd/system/

# Check if keys file exists and copy it
if [ -f "${KEYS_FILE}" ]; then
  echo "Copying ${KEYS_FILE} to ${INSTALL_DIR}..."
  cp ${KEYS_FILE} ${INSTALL_DIR}/
else
  echo "WARNING: ${KEYS_FILE} not found. You need to create this file at ${INSTALL_DIR}/${KEYS_FILE} with your API keys."
  echo "Format: One API key per line. Lines starting with # are ignored."
  touch ${INSTALL_DIR}/${KEYS_FILE}
fi

# Set permissions
echo "Setting permissions..."
chmod 755 ${INSTALL_DIR}/${BINARY_NAME}
chmod 644 ${INSTALL_DIR}/${KEYS_FILE}
chmod 644 /etc/systemd/system/${SERVICE_FILE}

# Reload systemd
echo "Reloading systemd daemon..."
systemctl daemon-reload

# Enable and start service
echo "Enabling and starting ${SERVICE_NAME} service..."
systemctl enable ${SERVICE_NAME}
systemctl start ${SERVICE_NAME}

# Check service status
echo "Checking service status..."
systemctl status ${SERVICE_NAME}

echo ""
echo "Installation complete!"
echo "The geminiproxy service is now running as a system service."
echo ""
echo "You can manage it with the following commands:"
echo "  - Check status: sudo systemctl status ${SERVICE_NAME}"
echo "  - Start service: sudo systemctl start ${SERVICE_NAME}"
echo "  - Stop service: sudo systemctl stop ${SERVICE_NAME}"
echo "  - Restart service: sudo systemctl restart ${SERVICE_NAME}"
echo "  - View logs: sudo journalctl -u ${SERVICE_NAME}"
echo ""
echo "The proxy is accessible at: http://localhost:8081"
echo ""
echo "Don't forget to add your API keys to ${INSTALL_DIR}/${KEYS_FILE} if you haven't already."
