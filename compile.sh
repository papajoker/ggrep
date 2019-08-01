#!/usr/bin/bash

# -o ggrepp falcutatif si main.go est dans le dossier "ggrep" ...

application="ggrep"
version="$(git describe --abbrev=0 2>/dev/null)"
if [[ -z "$version" ]]; then
    echo "no git version :("
    version="$1"
    if [[ -z "$version" ]]; then
        echo "TODO: pass Version by first parameter..."
        exit 9
    fi
fi

echo "$version"
commit="$(git rev-parse --short HEAD 2>/dev/null)"
echo "$commit"
git branch --show-current
date +%F

go build  \
    -ldflags \
    "-s -w
    -X main.GitID=${commit} \
    -X main.GitBranch=$(git branch --show-current 2>/dev/null) \
    -X main.Version=${version} \
    -X main.BuildDate=$(date +%F)" \
    -o ${application} && upx ${application}

echo ""
sleep 1 && eval "./${application} -h"
