#!/bin/bash

export TAG="V1.0.9.3" # Release TAG in GitHub
export Release1="v1.0.9.3" # Release Number
export buildpath="/Users/fabio.gos_sonarsource/Documents/GitHub/build-golc-v1.0.9.3"  # Replace with the path where the release files are located

# Use GITHUB_TOKEN from environment (set before running: export GITHUB_TOKEN=ghp_...)
if [ -z "$GITHUB_TOKEN" ]; then
  echo "Error: GITHUB_TOKEN is not set. Set it with: export GITHUB_TOKEN=your_token"
  exit 1
fi
GITHUB_ORG="SonarSource-Demos"    # Replace with your organization name
GITHUB_REPO="sonar-golc"   # Replace with the name of your GitHub repository

# Set a description for the release
RELEASE_DESCRIPTION="Added support for multiple groups in GitLab\n\
Fixed bug in GitLab nested groups\n\
Added support for new languages and file types, including Dart, Rust, JSON, Shell, Docker, and VB6, with appropriate comment syntaxes and file extensions.\n\
Deprecated Docker images\n"

CMD=`PWD`

# Function create release
create_release() {
  local tag="$1"
  local name="$2"
  local body="$3"

  echo "Création d'une nouvelle release avec le tag '$tag'..."
  curl -s -L -X POST -H "Authorization: token $GITHUB_TOKEN" \
    -H "Content-Type: application/json" \
    "https://api.github.com/repos/$GITHUB_ORG/$GITHUB_REPO/releases" -d "{
      \"tag_name\": \"$tag\",
      \"target_commitish\": \"main\",
      \"name\": \"$name\",
      \"body\": \"$body\",
      \"draft\": false,
      \"prerelease\": false
    }" > /dev/null

  echo "Release created."
}

# Function to retrieve the ID of the existing asset
find_asset_id() {
  local asset_name="$1"
  local release_id="$2"
  
  echo $(curl -s -L -H "Authorization: token $GITHUB_TOKEN" \
    "https://api.github.com/repos/$GITHUB_ORG/$GITHUB_REPO/releases/$release_id/assets" | \
    jq -r ".[] | select(.name == \"$asset_name\") | .id")
}

# Function to delete an existing asset
delete_asset() {
  local asset_id="$1"

  echo "Suppression de l'asset existant..."
  curl -s -X DELETE -H "Authorization: token $GITHUB_TOKEN" \
    "https://api.github.com/repos/$GITHUB_ORG/$GITHUB_REPO/releases/assets/$asset_id"
  echo "Asset deleted."
}

# Function to upload a file to the release
upload_asset() {
  local upload_url="$1"
  local file_path="$2"

  echo "Uploader le fichier : $(basename "$file_path")..."
  curl -s -X POST "$upload_url?name=$(basename "$file_path")" \
    -H "Authorization: token $GITHUB_TOKEN" \
    -H "Content-Type: application/zip" \
    --data-binary @"$file_path"
  echo "File uploaded to release successfully."
}


# Function update description
update_release_description() {
  local release_id="$1"
  local new_body="$2"

  echo "Updated release description..."
  curl -s -L -X PATCH -H "Authorization: token $GITHUB_TOKEN" \
    -H "Content-Type: application/json" \
    "https://api.github.com/repos/$GITHUB_ORG/$GITHUB_REPO/releases/$release_id" -d "{
      \"body\": \"$new_body\"
    }"
  
  echo "Description updated."
}

#----------------------- Begin Build --------------------------------#

# Buil arm64 Darwin

export GOARCH=arm64
export GOOS=darwin
export DEST=${buildpath}${Release1}/${GOARCH}/${GOOS}/golc_${Release1}_${GOOS}_${GOARCH}
export FILE_DEST=golc_${Release1}_${GOOS}_${GOARCH}

mkdir -p $DEST


# Build with proper tags and handle Windows .exe extension
# -trimpath removes file system paths from binaries for security/privacy
if [ "${GOOS}" = "windows" ]; then
    go build -trimpath -tags=golc -ldflags "-X main.version=${TAG}" -o ${DEST}/golc.exe golc.go
    go build -trimpath -tags=resultsall -o ${DEST}/ResultsAll.exe ResultsAll.go
else
    go build -trimpath -tags=golc -ldflags "-X main.version=${TAG}" -o ${DEST}/golc golc.go
    go build -trimpath -tags=resultsall -o ${DEST}/ResultsAll ResultsAll.go
fi
cp README.md  ${DEST}/
cp LICENSE ${DEST}/
cp -r imgs ${DEST}/
cp -r dist ${DEST}/
cp config_sample.json ${DEST}/config.json
cd ${buildpath}${Release1}/${GOARCH}/${GOOS}/
zip -r ${FILE_DEST}.zip ${FILE_DEST}
cd $CMD

# Buil arm64 Linux

export GOOS=linux
export DEST=${buildpath}${Release1}/${GOARCH}/${GOOS}/golc_${Release1}_${GOOS}_${GOARCH}
export FILE_DEST=golc_${Release1}_${GOOS}_${GOARCH}

mkdir -p $DEST

# Build with proper tags and handle Windows .exe extension
# -trimpath removes file system paths from binaries for security/privacy
if [ "${GOOS}" = "windows" ]; then
    go build -trimpath -tags=golc -ldflags "-X main.version=${TAG}" -o ${DEST}/golc.exe golc.go
    go build -trimpath -tags=resultsall -o ${DEST}/ResultsAll.exe ResultsAll.go
else
    go build -trimpath -tags=golc -ldflags "-X main.version=${TAG}" -o ${DEST}/golc golc.go
    go build -trimpath -tags=resultsall -o ${DEST}/ResultsAll ResultsAll.go
fi
cp README.md  ${DEST}/
cp LICENSE ${DEST}/
cp -r imgs ${DEST}/
cp -r dist ${DEST}/
cp config_sample.json ${DEST}/config.json
cd ${buildpath}${Release1}/${GOARCH}/${GOOS}/
zip -r ${FILE_DEST}.zip ${FILE_DEST}
cd $CMD

# Buil arm64 Windows

export GOOS=windows
export DEST=${buildpath}${Release1}/${GOARCH}/${GOOS}/golc_${Release1}_${GOOS}_${GOARCH}
export FILE_DEST=golc_${Release1}_${GOOS}_${GOARCH}

mkdir -p $DEST

# Build with proper tags and handle Windows .exe extension
# -trimpath removes file system paths from binaries for security/privacy
if [ "${GOOS}" = "windows" ]; then
    go build -trimpath -tags=golc -ldflags "-X main.version=${TAG}" -o ${DEST}/golc.exe golc.go
    go build -trimpath -tags=resultsall -o ${DEST}/ResultsAll.exe ResultsAll.go
else
    go build -trimpath -tags=golc -ldflags "-X main.version=${TAG}" -o ${DEST}/golc golc.go
    go build -trimpath -tags=resultsall -o ${DEST}/ResultsAll ResultsAll.go
fi
cp README.md  ${DEST}/
cp LICENSE ${DEST}/
cp -r imgs ${DEST}/
cp -r dist ${DEST}/
cp config_sample.json ${DEST}/config.json
cd ${buildpath}${Release1}/${GOARCH}/${GOOS}/
zip -r ${FILE_DEST}.zip ${FILE_DEST}
cd $CMD

# Buil amd64 Darwin

export GOARCH=amd64
export GOOS=darwin
export DEST=${buildpath}${Release1}/${GOARCH}/${GOOS}/golc_${Release1}_${GOOS}_${GOARCH}
export FILE_DEST=golc_${Release1}_${GOOS}_${GOARCH}

mkdir -p $DEST

# Build with proper tags and handle Windows .exe extension
# -trimpath removes file system paths from binaries for security/privacy
if [ "${GOOS}" = "windows" ]; then
    go build -trimpath -tags=golc -ldflags "-X main.version=${TAG}" -o ${DEST}/golc.exe golc.go
    go build -trimpath -tags=resultsall -o ${DEST}/ResultsAll.exe ResultsAll.go
else
    go build -trimpath -tags=golc -ldflags "-X main.version=${TAG}" -o ${DEST}/golc golc.go
    go build -trimpath -tags=resultsall -o ${DEST}/ResultsAll ResultsAll.go
fi
cp README.md  ${DEST}/
cp LICENSE ${DEST}/
cp -r imgs ${DEST}/
cp -r dist ${DEST}/
cp config_sample.json ${DEST}/config.json
cd ${buildpath}${Release1}/${GOARCH}/${GOOS}/
zip -r ${FILE_DEST}.zip ${FILE_DEST}
cd $CMD

# Buil amd64 Linux

export GOOS=linux
export DEST=${buildpath}${Release1}/${GOARCH}/${GOOS}/golc_${Release1}_${GOOS}_${GOARCH}
export FILE_DEST=golc_${Release1}_${GOOS}_${GOARCH}

mkdir -p $DEST

# Build with proper tags and handle Windows .exe extension
# -trimpath removes file system paths from binaries for security/privacy
if [ "${GOOS}" = "windows" ]; then
    go build -trimpath -tags=golc -ldflags "-X main.version=${TAG}" -o ${DEST}/golc.exe golc.go
    go build -trimpath -tags=resultsall -o ${DEST}/ResultsAll.exe ResultsAll.go
else
    go build -trimpath -tags=golc -ldflags "-X main.version=${TAG}" -o ${DEST}/golc golc.go
    go build -trimpath -tags=resultsall -o ${DEST}/ResultsAll ResultsAll.go
fi
cp README.md  ${DEST}/
cp LICENSE ${DEST}/
cp -r imgs ${DEST}/
cp -r dist ${DEST}/
cp config_sample.json ${DEST}/config.json
cd ${buildpath}${Release1}/${GOARCH}/${GOOS}/
zip -r ${FILE_DEST}.zip ${FILE_DEST}
cd $CMD

# Buil amd64 Windows

export GOOS=windows
export DEST=${buildpath}${Release1}/${GOARCH}/${GOOS}/golc_${Release1}_${GOOS}_${GOARCH}
export FILE_DEST=golc_${Release1}_${GOOS}_${GOARCH}

mkdir -p $DEST

# Build with proper tags and handle Windows .exe extension
# -trimpath removes file system paths from binaries for security/privacy
if [ "${GOOS}" = "windows" ]; then
    go build -trimpath -tags=golc -ldflags "-X main.version=${TAG}" -o ${DEST}/golc.exe golc.go
    go build -trimpath -tags=resultsall -o ${DEST}/ResultsAll.exe ResultsAll.go
else
    go build -trimpath -tags=golc -ldflags "-X main.version=${TAG}" -o ${DEST}/golc golc.go
    go build -trimpath -tags=resultsall -o ${DEST}/ResultsAll ResultsAll.go
fi
cp README.md  ${DEST}/
cp LICENSE ${DEST}/
cp -r imgs ${DEST}/
cp -r dist ${DEST}/
cp config_sample.json ${DEST}/config.json
cd ${buildpath}${Release1}/${GOARCH}/${GOOS}/
zip -r ${FILE_DEST}.zip ${FILE_DEST}
cd $CMD

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

# Begin to push Releae in GitHub Repository

# Retrieve information from existing release
RELEASE_RESPONSE=$(curl -s -L -H "Authorization: token $GITHUB_TOKEN" \
  "https://api.github.com/repos/$GITHUB_ORG/$GITHUB_REPO/releases/tags/$TAG")

if [[ $(echo "$RELEASE_RESPONSE" | jq -r '.message') == "Not Found" ]]; then
  echo "The release for tag '$TAG' does not exist. Creating release..."
  create_release "$TAG" "$TAG" "$RELEASE_DESCRIPTION"
  # Retrieve information from the newly created release
  RELEASE_RESPONSE=$(curl -s -L -H "Authorization: token $GITHUB_TOKEN" \
    "https://api.github.com/repos/$GITHUB_ORG/$GITHUB_REPO/releases/tags/$TAG")
fi

# Retrieve the upload URL and release ID
UPLOAD_URL=$(echo "$RELEASE_RESPONSE" | jq -r '.upload_url' | sed "s/{?name,label}//")
RELEASE_ID=$(echo "$RELEASE_RESPONSE" | jq -r '.id')

# If we still don't have a valid release (e.g. API returned redirect/error), try creating it
if [ -z "$RELEASE_ID" ] || [ "$RELEASE_ID" = "null" ] || [ -z "$UPLOAD_URL" ] || [ "$UPLOAD_URL" = "null" ]; then
  echo "Could not get release (tag may not exist). Creating release..."
  create_release "$TAG" "$TAG" "$RELEASE_DESCRIPTION"
  RELEASE_RESPONSE=$(curl -s -L -H "Authorization: token $GITHUB_TOKEN" \
    "https://api.github.com/repos/$GITHUB_ORG/$GITHUB_REPO/releases/tags/$TAG")
  UPLOAD_URL=$(echo "$RELEASE_RESPONSE" | jq -r '.upload_url' | sed "s/{?name,label}//")
  RELEASE_ID=$(echo "$RELEASE_RESPONSE" | jq -r '.id')
fi

# Only update description if we got a valid release ID
if [ -n "$RELEASE_ID" ] && [ "$RELEASE_ID" != "null" ]; then
  update_release_description "$RELEASE_ID" "$RELEASE_DESCRIPTION"
else
  echo "Warning: Could not get release ID (API may have redirected). Skipping description update."
fi

# Bail out if we don't have a valid release (uploads would fail)
if [ -z "$UPLOAD_URL" ] || [ "$UPLOAD_URL" = "null" ] || [ -z "$RELEASE_ID" ] || [ "$RELEASE_ID" = "null" ]; then
  echo "Error: Could not get release or upload URL. Check GITHUB_TOKEN and repo access."
  exit 1
fi

# Retrieve the list of files from the release
ASSETS_RESPONSE=$(curl -s -L -H "Authorization: token $GITHUB_TOKEN" \
  "https://api.github.com/repos/$GITHUB_ORG/$GITHUB_REPO/releases/$RELEASE_ID/assets")


declare -a GOARCH_VALUES=("amd64" "arm64")
declare -a GOOS_VALUES=("linux" "windows" "darwin")

# Upload zip files to each directory for each combination of GOARCH and GOOS
for GOARCH in "${GOARCH_VALUES[@]}"; do
    for GOOS in "${GOOS_VALUES[@]}"; do
        zip_file="${buildpath}${Release1}/${GOARCH}/${GOOS}/golc_${Release1}_${GOOS}_${GOARCH}.zip"
        
       # Find the ID of the existing asset with the same name
  
        EXISTING_ASSET_ID=$(echo "$ASSETS_RESPONSE" | jq -r ".[] | select(.name == \"$(basename $zip_file)\") | .id")

        # Delete existing asset, if found
        if [ ! -z "$EXISTING_ASSET_ID" ]; then
            delete_asset "$EXISTING_ASSET_ID"
        fi
        upload_asset "$UPLOAD_URL" "$zip_file"
    done
done

# Upload source code archives
if [ -f "${buildpath}${Release1}/source.zip" ]; then
    EXISTING_ASSET_ID=$(echo "$ASSETS_RESPONSE" | jq -r ".[] | select(.name == \"source.zip\") | .id")
    if [ ! -z "$EXISTING_ASSET_ID" ]; then
        delete_asset "$EXISTING_ASSET_ID"
    fi
    upload_asset "$UPLOAD_URL" "${buildpath}${Release1}/source.zip"
fi

if [ -f "${buildpath}${Release1}/source.tar.gz" ]; then
    EXISTING_ASSET_ID=$(echo "$ASSETS_RESPONSE" | jq -r ".[] | select(.name == \"source.tar.gz\") | .id")
    if [ ! -z "$EXISTING_ASSET_ID" ]; then
        delete_asset "$EXISTING_ASSET_ID"
    fi
    # Note: GitHub API expects correct Content-Type for tar.gz
    curl -s -X POST "$UPLOAD_URL?name=source.tar.gz" \
        -H "Authorization: token $GITHUB_TOKEN" \
        -H "Content-Type: application/gzip" \
        --data-binary @"${buildpath}${Release1}/source.tar.gz"
    echo "Source code tar.gz uploaded to release successfully."
fi

cd $CMD


