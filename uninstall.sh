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
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

# Stop and disable service
echo "Stopping and disabling ${SERVICE_NAME} service..."
systemctl stop ${SERVICE_NAME} || true
systemctl disable ${SERVICE_NAME} || true

# Remove service file
echo "Removing service file..."
if [ -f "${SERVICE_FILE}" ]; then
  rm ${SERVICE_FILE}
fi

# Reload systemd
echo "Reloading systemd daemon..."
systemctl daemon-reload

# Ask if user wants to remove installation directory
read -p "Do you want to remove the installation directory (${INSTALL_DIR})? [y/N] " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
  echo "Removing installation directory..."
  rm -rf ${INSTALL_DIR}
  echo "Installation directory removed."
else
  echo "Installation directory kept at ${INSTALL_DIR}."
fi

echo ""
echo "Uninstallation complete!"
echo "The geminiproxy service has been removed from your system."
