@echo off
if not exist go.work (
    echo Initializing Go workspace...
    go work init
)
echo Adding packages to Go workspace...
go work use ./packages/ncm-dumper ./packages/net-inspect ./packages/LFS
echo Workspace initialized successfully.
