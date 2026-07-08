#!/bin/bash
if [ ! -f go.work ]; then
    echo "Initializing Go workspace..."
    go work init
fi
echo "Adding packages to Go workspace..."
go work use ./packages/ncm-dumper ./packages/net-inspect ./packages/LFS ./packages/game-prioritizer
echo "Workspace initialized successfully."
