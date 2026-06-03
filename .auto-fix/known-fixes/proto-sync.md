# Proto Sync Fix Template

## Pattern
- `Proto generated files are out of date`
- Wire/Proto CI check failure

## Fix Strategy
1. Run `make api` to regenerate proto files
2. Run `make wire` to regenerate wire files
3. Commit the regenerated files

## Commands
```bash
make api && make wire
git add api/ web/src/services/ cmd/admin/wire_gen.go
git commit -m "chore: sync generated files"
```
