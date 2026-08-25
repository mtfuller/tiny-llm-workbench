---
name: new-command
description: Scaffold a new Cobra CLI command (with test) following this repo's conventions. Use when adding a new subcommand to the tiny-llm-workbench (`tlw`) CLI, e.g. "add a models command", "create a new subcommand for X".
---

# New Cobra command

Scaffold a new top-level or nested Cobra command consistent with this repo's existing commands
(`cmd/greet.go`, `cmd/calc.go`, `cmd/process.go`, `cmd/version.go`) and
[STYLEGUIDE.instructions.md](../../../.github/instructions/STYLEGUIDE.instructions.md).

## Steps

1. **Confirm the command name and parent.** Most commands attach to `rootCmd` in `cmd/root.go`. If this
   command belongs under an existing command (e.g. a subcommand of a future `models` command), attach to
   that command's variable instead.
2. **Create `cmd/<name>.go`** following this shape:

   ```go
   package cmd

   import (
       "github.com/mtfuller/tiny-llm-workbench/internal/color"
       "github.com/spf13/cobra"
   )

   var <name>Cmd = &cobra.Command{
       Use:   "<name> [args]",
       Short: "<one-line description>",
       Long:  `<longer description, optional>`,
       Args:  cobra.ExactArgs(0), // pick the right validator: ExactArgs, MinimumNArgs, MaximumNArgs, ...
       RunE: func(cmd *cobra.Command, args []string) error {
           // business logic goes in internal/ or pkg/, not here — this should stay thin
           color.Success("...")
           return nil
       },
   }

   func init() {
       rootCmd.AddCommand(<name>Cmd)
       // <name>Cmd.Flags().StringVarP(&someVar, "flag", "f", "default", "description")
   }
   ```

   - Use `RunE` (not `Run`) so errors propagate instead of requiring manual `os.Exit`.
   - Keep the `Run`/`RunE` body thin — real logic belongs in `internal/<pkg>/` (or `pkg/` if it's
     reusable outside this CLI), unit-tested there. The command file just wires flags/args to that
     logic and formats output with `internal/color`.
   - Use `internal/logger` for structured/debug logging, `internal/color` for user-facing output
     (`color.Success`, `color.Error`, `color.Info`, `color.Warn`).
   - Always fill in `Short` (and `Long` for anything non-trivial) — `task build` output and `--help` both
     depend on it.

3. **Add flags in `init()`**, bound with `cmd.Flags().StringVarP(...)` etc. Use
   `cmd.MarkFlagRequired(...)` for required flags.
4. **Write `cmd/<name>_test.go`** if the command has meaningful branching logic worth unit testing
   directly, and add a case to `tests/integration_test.go` that execs the built CLI with this command
   per [TESTING.instructions.md](../../../.github/instructions/TESTING.instructions.md).
5. **Update README.md** if this is a user-facing command worth documenting under "Available Commands".
6. Run `task build && task test` to confirm it compiles, is registered, and passes.

## Module path

Import paths in the example above use `github.com/mtfuller/tiny-llm-workbench`, matching `go.mod`.
