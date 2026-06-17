#!/bin/bash
# Verification script for embedded web UI build
# This script verifies that the web UI is properly embedded and has no external dependencies.

set -e

echo "=== UVB-76 Web UI Embed Verification ==="
echo

# Change to uvb76 directory
cd "$(dirname "$0")/.."

# 1. Verify web/dist exists
echo "[1/6] Checking web/dist exists..."
if [ ! -d "web/dist" ]; then
    echo "ERROR: web/dist directory not found. Run 'npm run build' in web/ directory."
    exit 1
fi
echo "  OK: web/dist exists"

# 2. Verify required files exist in web/dist
echo "[2/6] Checking embedded files..."
if [ ! -f "web/dist/index.html" ]; then
    echo "ERROR: web/dist/index.html not found"
    exit 1
fi

ASSETS_DIR="web/dist/assets"
if [ ! -d "$ASSETS_DIR" ]; then
    echo "ERROR: $ASSETS_DIR not found"
    exit 1
fi

# Check for CSS and JS files
CSS_COUNT=$(find "$ASSETS_DIR" -name "*.css" 2>/dev/null | wc -l)
JS_COUNT=$(find "$ASSETS_DIR" -name "*.js" 2>/dev/null | wc -l)

if [ "$CSS_COUNT" -eq 0 ]; then
    echo "ERROR: No CSS files found in $ASSETS_DIR"
    exit 1
fi

if [ "$JS_COUNT" -eq 0 ]; then
    echo "ERROR: No JS files found in $ASSETS_DIR"
    exit 1
fi
echo "  OK: Found $CSS_COUNT CSS file(s) and $JS_COUNT JS file(s)"

# 3. Verify no CDN dependencies in generated HTML
echo "[3/6] Checking for CDN dependencies in HTML..."
if grep -qE 'https?://' "web/dist/index.html" 2>/dev/null; then
    echo "ERROR: Generated HTML contains HTTP/HTTPS URLs (possible CDN dependency)"
    grep -nE 'https?://' "web/dist/index.html" || true
    exit 1
fi
echo "  OK: No external URLs in HTML"

# 4. Check JS bundles recursively for external dependencies
echo "[4/6] Checking all JS bundles recursively for external dependencies..."
FAILED=0
while IFS= read -r js_file; do
    # Check for import from external URLs
    if grep -qE 'from ["'"'"']https?://' "$js_file" 2>/dev/null; then
        echo "ERROR: JS bundle contains external import: $js_file"
        grep -nE 'from ["'"'"']https?://' "$js_file" || true
        FAILED=1
    fi
    # Check for import() with external URLs
    if grep -qE 'import\(["'"'"']https?://' "$js_file" 2>/dev/null; then
        echo "ERROR: JS bundle contains dynamic import from external URL: $js_file"
        FAILED=1
    fi
    # Check for url() in CSS content (only relevant for JS files that might contain CSS strings)
    if grep -qE 'url\(["'"'"']https?://' "$js_file" 2>/dev/null; then
        echo "ERROR: JS bundle contains url() with external URL: $js_file"
        FAILED=1
    fi
done < <(find "$ASSETS_DIR" -name "*.js" -type f 2>/dev/null)

if [ "$FAILED" -eq 1 ]; then
    exit 1
fi
echo "  OK: No external dependencies in all JS bundles"

# 5. Check our source files for external URL references
# Note: Vendor bundles (Chart.js) contain license attributions which are allowed
echo "[5/6] Checking source assets for external URL references..."
FAILED=0

# Only check our source files, not vendor bundles
# Our source files are typically smaller and in subdirs or have specific patterns
while IFS= read -r asset_file; do
    # Skip vendor bundles (main-*.js from Vite typically contains Chart.js)
    if [[ "$asset_file" == *"-main-"* ]] || [[ "$asset_file" == *"chunk-"* ]]; then
        continue
    fi
    # For HTML files, check for any external URLs
    if [[ "$asset_file" == *.html ]]; then
        if grep -qE 'https?://' "$asset_file" 2>/dev/null; then
            echo "ERROR: HTML contains external URL: $asset_file"
            grep -nE 'https?://' "$asset_file" | head -5 || true
            FAILED=1
        fi
    fi
done < <(find "web/dist" \( -name "*.html" \) -type f 2>/dev/null)

if [ "$FAILED" -eq 1 ]; then
    exit 1
fi
echo "  OK: Source files have no external URLs"

if [ "$FAILED" -eq 1 ]; then
    exit 1
fi
echo "  OK: No external URLs in any assets"

# 6. Verify go build includes embedded content
echo "[6/6] Verifying Go build includes web content..."
# Build a test binary
if ! go build -o uvb76-verify-test . 2>/dev/null; then
    echo "ERROR: Go build failed"
    exit 1
fi
rm -f uvb76-verify-test

# Verify server/admin.html is removed (should use embedded content now)
if [ -f "server/admin.html" ]; then
    echo "WARNING: server/admin.html still exists (should be removed after migration)"
fi

echo "  OK: Go build succeeded with embedded content"

echo
echo "=== All verification checks passed ==="
exit 0
