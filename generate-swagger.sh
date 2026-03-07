#!/bin/bash
# Add Go bin to PATH if not already present
export PATH="$PATH:$(go env GOPATH)/bin"

# Navigate to project directory
cd "$(dirname "$0")"

# Generate Swagger documentation
echo "Generating Swagger documentation..."
swag init -g cmd/server/main.go -d ./

if [ $? -eq 0 ]; then
    echo "✅ Swagger documentation generated successfully!"
    echo ""
    echo "📖 Start your server with:"
    echo "   make server"
    echo ""
    echo "🌐 Then access the Swagger UI at:"
    echo "   http://localhost:8080/docs"
else
    echo "❌ Failed to generate Swagger documentation"
    exit 1
fi
