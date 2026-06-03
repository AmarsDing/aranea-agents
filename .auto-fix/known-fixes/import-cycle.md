# Import Cycle Fix Template

## Pattern
- `import cycle not allowed`
- Circular dependency between packages

## Fix Strategy
1. Extract shared types into a separate package
2. Use interfaces to break the cycle
3. Move the dependency to a higher-level package
4. Use dependency injection instead of direct import

## Example
```go
// Before (cycle: A imports B, B imports A)
package a
import "project/b"

package b
import "project/a"

// After (extract shared types)
package a
import "project/types"

package b
import "project/types"
```
