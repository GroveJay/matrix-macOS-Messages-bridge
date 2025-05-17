#!/bin/sh
MAUTRIX_VERSION=$(cat go.mod | grep 'maunium.net/go/mautrix ' | awk '{ print $2 }' | head -n1)
echo "MAUTRIX_VERSION: ${MAUTRIX_VERSION}"
GO_LDFLAGS="-s -w -X main.Tag=$(git describe --exact-match --tags 2>/dev/null) -X main.Commit=$(git rev-parse HEAD) -X 'main.BuildTime=`date -Iseconds`' -X 'maunium.net/go/mautrix.GoModVersion=$MAUTRIX_VERSION'"
echo "GO_LDFLAGS: ${GO_LDFLAGS}"
echo "Additional agruments: $@"
go build -ldflags="$GO_LDFLAGS" -o bridge ./cmd "$@"