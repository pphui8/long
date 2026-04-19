# Exit on error
set -e

echo "--- Starting Environment Setup for Debian 13.3 ---"

# 1. Update system packages
sudo apt update && sudo apt upgrade -y
sudo apt install -y curl wget git build-essential ca-certificates

# 2. Install Node.js v22.17.0
# We use the official binary distribution for a specific version match
echo "Installing Node.js v22.17.0..."
NODE_VERSION="v22.17.0"
DISTRO="linux-x64"
wget https://nodejs.org/dist/$NODE_VERSION/node-$NODE_VERSION-$DISTRO.tar.xz
sudo mkdir -p /usr/local/lib/nodejs
sudo tar -xJvf node-$NODE_VERSION-$DISTRO.tar.xz -C /usr/local/lib/nodejs 
# Set up symlinks for global access
sudo ln -sf /usr/local/lib/nodejs/node-$NODE_VERSION-$DISTRO/bin/node /usr/bin/node
sudo ln -sf /usr/local/lib/nodejs/node-$NODE_VERSION-$DISTRO/bin/npm /usr/bin/npm
sudo ln -sf /usr/local/lib/nodejs/node-$NODE_VERSION-$DISTRO/bin/npx /usr/bin/npx

# 3. Install Go 1.26.2
echo "Installing Go 1.26.2..."
GO_VERSION="1.26.2"
wget https://golang.org/dl/go$GO_VERSION.linux-amd64.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go$GO_VERSION.linux-amd64.tar.gz

# 4. Configure Environment Variables
echo "Configuring PATH..."
# Add Go to system-wide profile
echo 'export PATH=$PATH:/usr/local/go/bin' | sudo tee /etc/profile.d/go.sh
# Add Go bin and Node bin to current user's .bashrc
echo 'export GOPATH=$HOME/go' >> $HOME/.bashrc
echo 'export PATH=$PATH:/usr/local/go/bin:$GOPATH/bin' >> $HOME/.bashrc

# 5. Install and configure Nginx
sudo apt install -y nginx
sudo systemctl enable nginx
sudo systemctl start nginx

# 6. Install Certbot for SSL certificates
sudo apt install -y certbot python3-certbot-nginx

# 999. Cleanup
rm node-$NODE_VERSION-$DISTRO.tar.xz
rm go$GO_VERSION.linux-amd64.tar.gz

echo "--- Setup Complete ---"
echo "Please run: 'source ~/.bashrc' to refresh your current session."

sudo certbot --nginx -d pphui8.com -d llm.pphui8.com