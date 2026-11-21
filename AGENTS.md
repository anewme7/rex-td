# rextd project rules

## Product and integration

- Build around one workflow. Find the task, see what needs a person, jump to the right Rex pane, then return to the task and its next step.
- Use Rex's public actions, events and Program Status Protocol for hosts, sessions, pane state and navigation. Use official Agent APIs or local provider interfaces for session details and quota. Reuse existing adapters and supported dependencies before adding a collector, hook, runtime or service.
- Scope live subscriptions to the server or view that needs them. Coalesce updates, cancel work when its view closes and cap concurrency, retries, caches and logs. Keep task data and explicit links durable. Store only the minimum live metadata required to restore user context.
- Keep Tasks and Attention quiet. Lead with the task title and a short next step or blocker. Show host, session, pane and Agent detail as the user drills in. Use spacing and indentation for grouping. Reserve color for selection, meaningful status and user-chosen accents. Make common actions work with both keyboard and pointer input.
- Adopt a reference project's behavior when it removes work from the task flow. Document the native source of truth and measure the change with real Rex panes. Compare key response time, idle CPU, resident memory, subprocess starts, remote calls and disk growth when relevant to the change.

## Release versions

- The project owner chooses the release version. Keep the current version unless the owner explicitly approves a new version number. Bug fixes, documentation updates, builds, and merges do not authorize a version bump.
- Publishing a release or replacing an existing one requires the owner's approval. Use the approved tag for the GitHub release, packaged binaries, and local sidebar installation. Verify the public download and `rextd --version` before reporting the release as complete.
- Group small fixes for the same change into one commit before merging. Rewriting public history or replacing a published tag requires explicit approval.
