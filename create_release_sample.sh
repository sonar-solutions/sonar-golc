#!/bin/bash

export TAG="ver2.1.0" # Release TAG in GitHub
export Release1="ver2.1.0" # Release Number
export buildpath="XXXXXXX"  # Replace with the path where the release files are located

CMD=`PWD`

#----------------------- Begin Build --------------------------------#

build_platform() {
  local GOARCH="$1"
  local GOOS="$2"

  export GOARCH GOOS
  export DEST=${buildpath}${Release1}/${GOARCH}/${GOOS}/golc_${Release1}_${GOOS}_${GOARCH}
  export FILE_DEST=golc_${Release1}_${GOOS}_${GOARCH}

  mkdir -p "$DEST"

  # -trimpath removes file system paths from binaries for security/privacy
  if [ "${GOOS}" = "windows" ]; then
      go build -trimpath -tags=launcher -ldflags "-X github.com/SonarSource-Demos/sonar-golc/assets.Version=${TAG}" -o "${DEST}/golc-launcher.exe" golc-launcher.go golc.go
      go build -trimpath -tags=resultsall -ldflags "-X github.com/SonarSource-Demos/sonar-golc/assets.Version=${TAG}" -o "${DEST}/ResultsAll.exe" ResultsAll.go
  else
      go build -trimpath -tags=launcher -ldflags "-X github.com/SonarSource-Demos/sonar-golc/assets.Version=${TAG}" -o "${DEST}/golc-launcher" golc-launcher.go golc.go
      go build -trimpath -tags=resultsall -ldflags "-X github.com/SonarSource-Demos/sonar-golc/assets.Version=${TAG}" -o "${DEST}/ResultsAll" ResultsAll.go
  fi

  cp README.md  "${DEST}/"
  cp LICENSE    "${DEST}/"
  cp -r imgs    "${DEST}/"
  cp config_sample.json "${DEST}/config.json"

  cd "${buildpath}${Release1}/${GOARCH}/${GOOS}/"
  zip -r "${FILE_DEST}.zip" "${FILE_DEST}"
  cd "$CMD"
  return 0
}

build_platform arm64 darwin
build_platform arm64 linux
build_platform arm64 windows
build_platform amd64 darwin
build_platform amd64 linux
build_platform amd64 windows

#------------------------------ End Build ------------------------------------#

# Create source code archives
echo "Creating source code archives..."

SOURCE_DIR="${buildpath}${Release1}/source"
mkdir -p ${SOURCE_DIR}

# Use git archive for clean source (excludes .gitignore files)
if command -v git &> /dev/null && git rev-parse --git-dir > /dev/null 2>&1; then
    git archive HEAD --prefix=sonar-golc-${Release1}/ | tar -x -C ${SOURCE_DIR}

    # Create source.zip
    cd ${SOURCE_DIR}
    zip -r ../source.zip sonar-golc-${Release1}/
    cd $CMD

    # Create source.tar.gz
    cd ${SOURCE_DIR}
    tar -czf ../source.tar.gz sonar-golc-${Release1}/
    cd $CMD

    echo "✓ Created source.zip and source.tar.gz"
else
    echo "Warning: Not a git repository, skipping source archives"
fi

echo ""
echo "Build complete. Archives written to: ${buildpath}${Release1}/"
