# rextd user guide

rextd adds a task list and an Attention view to Rex. Use it to keep track of work, see which agents need input, and jump back to a pane.

## Install and open the sidebar

1. Install Rex and make its `rex` command available on the machine running your terminal. Download the Apple Silicon or Linux x86-64 package and matching checksum from [GitHub Releases](https://github.com/anewme7/rextd/releases). The archive contains the `rextd` executable.
2. Follow the [checksum and extraction steps](../README.md#install-and-open-the-sidebar) for your platform. The package bundles SQLite.
3. In a Rex terminal, change to the extracted directory and run:

   ```sh
   rex run --split=right --ratio 35 --shell=none --label rextd -- "$PWD/rextd"
   ```

The sidebar starts in English. Press **Shift+L** to switch between English and Simplified Chinese. To start in Chinese, append `--lang zh` after the executable in the launch command.

To try sample tasks and alerts first, run `./rextd --demo`. The demo uses fictional data in memory. `--demo` shows `演示` or **Demo**.

## Create a task and jump to a linked pane

1. Open **Tasks (任务)** and press `+`. Type a title such as “Review the release notes” and press Enter to save it.
2. Select the task and press Right to expand it. Press Right again to open details, or use `o` to open details directly.
3. Press lowercase `l` in details, choose a Rex pane, and press Enter to link it.
4. Select a linked pane in task details and press Enter to jump to it.

## Finish and organize tasks

Click the checkbox or press `x` to mark a task done or reopen it. Open the task menu with `m` to cancel, archive, reorder, or split work into subtasks. Press Shift+A to see the archive and `/` to search.

Load the [bundled Rex actions](../README.md#add-rextd-to-the-rex-command-palette). On macOS, press Shift+Command+P and search for **td** to open the board, find a task, or capture the current pane. After jumping, use **Open Task Board** to return to the selected task.

## Review suggested tasks

Open a pane's details from **Attention**, then press `d` to discover a candidate or `w` to watch the pane for suggestions. The sidebar uses Rex status updates from the Codex and Claude Code panes you choose.

On **Tasks**, expand **Candidates** and select a suggestion. Press Enter to review it and edit the title. Press Enter again to create a task with a link to the original pane. Press `i` to ignore a candidate's topic.

## Review agent activity

Select a numbered host with Alt+1 through Alt+9, or press Alt+0 to show all hosts. See the [Rex command palette setup](../README.md#add-rextd-to-the-rex-command-palette) for the optional action.

Attention shows approvals, questions, sign-ins, errors, and completed turns that Rex reports. Open [Agent session details](../README.md#agent-session-details) for the latest activity and available session metadata.

| To do this | Press |
| --- | --- |
| Group by session, agent, or host | `v` |
| Fold or expand the current group | `c` |
| Fold or expand a whole host in Host view | Shift+C |
| Inspect a pane's key events | Right |
| Mark a completed result as seen or undo the mark | `y` in pane details |
| Jump to the selected pane | Enter |
| Peek at status | Space |
| Pin a task or pane | `p` |
| See the shortcut list | `?` |

Press `a` in Attention to show all panes and reopen a result you marked as seen. Approvals stay in Attention until Rex reports a change.

Saved task pins persist. Attention pins last until you close the sidebar. For cross-host jumps, connect the Rex app to both servers, give the device a unique Rex client label, and enable Rex Remote Control for tab switches.

## Read machine and usage data

- **Hosts · CPU** shows the CPU percentage and alert count for each available host. Local CPU appears automatically on supported platforms. Follow [remote CPU setup](../README.md#enable-remote-cpu) to enable a remote host.
- **Usage** shows each provider's reported quota windows. When the heading is **5h/wk**, read the left value as five-hour usage and the right value as weekly usage. A dash means a reading is unavailable.
- For **Codex**, put `codex` on the sidebar machine's PATH and sign in. For **Claude Code**, follow the [status-line setup](../README.md#enable-claude-code-usage).

Click **Hosts** or **Usage**, or press `f`, for detail. Press Esc to return to the list.

## Back up, update, or remove

Tasks and pane links are saved to `~/.local/share/rex-td/tasks.sqlite3` by default, on the machine running the sidebar. If you set `REX_TD_DATA_DIR` or `XDG_DATA_HOME`, use the [data location rules](../README.md#data-updates-and-removal) to find it.

- **Back up** by quitting with `q` and copying the task database.
- **Update** by quitting with `q` and launching the binary from the newer archive. If you set up Rex actions, point `rextd_root` to the new directory and reload Rex.
- **Remove** by quitting with `q`, deleting the extracted program, and removing any optional Rex and Claude settings you added. Keep the database if you plan to use your tasks again.
