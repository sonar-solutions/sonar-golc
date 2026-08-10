#!/bin/bash

# Generate test coverage for local development
# Note: Coverage is automatically generated in CI/CD pipeline
set -e

echo "🔬 Generating test coverage reports for local development..."

# Clean up old coverage files
rm -f coverage-*.out coverage.out

# Create config file for testing if it doesn't exist
if [ ! -f config.json ]; then
    echo "📝 Creating config.json from sample for testing..."
    cp config_sample.json config.json
fi

# 1. Test pkg/ directory (shared packages)
echo "📦 Testing pkg/ directory..."
go test -coverprofile=coverage-pkg.out ./pkg/...

# 2. Test golc application (with build tag)  
echo "🔧 Testing golc application..."
go test -tags=engine -coverprofile=coverage-golc.out .

# 3. Test resultsall application (with build tag)
echo "🌐 Testing resultsall application..."
go test -tags=resultsall -coverprofile=coverage-resultsall.out .

# 4. Display coverage summary for each component
echo "📊 Coverage summary by component:"
echo "📦 pkg/ directory:"
go tool cover -func=coverage-pkg.out | tail -1

echo "🔧 golc application:"
go tool cover -func=coverage-golc.out | tail -1

echo "🌐 resultsall application:"  
go tool cover -func=coverage-resultsall.out | tail -1

# 5. Generate HTML reports (optional)
if [ "$1" = "--html" ]; then
    echo "🌐 Generating HTML coverage reports..."
    go tool cover -html=coverage-pkg.out -o coverage-pkg.html
    go tool cover -html=coverage-golc.out -o coverage-golc.html
    go tool cover -html=coverage-resultsall.out -o coverage-resultsall.html
    echo "✅ HTML reports generated:"
    echo "   - coverage-pkg.html"
    echo "   - coverage-golc.html" 
    echo "   - coverage-resultsall.html"
fi

echo "✅ Coverage reports generated successfully!"
echo "📋 Files created for SonarQube:"
echo "   - coverage-pkg.out (pkg/ directory)"  
echo "   - coverage-golc.out (golc application)"
echo "   - coverage-resultsall.out (resultsall application)"

echo ""
echo "💡 Usage:"
echo "   ./generate_coverage.sh          # Generate reports for local development"
echo "   ./generate_coverage.sh --html   # Generate reports + HTML views"
echo ""
echo "🎯 Note: Coverage is automatically generated in CI/CD pipeline"
echo "   Pipeline generates the same 3 files for SonarQube analysis"
