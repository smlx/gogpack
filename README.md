# gogpack

GOG game installer to Flatpak converter.

[![Release](https://github.com/smlx/go-cli-github/actions/workflows/release.yaml/badge.svg)](https://github.com/smlx/go-cli-github/actions/workflows/release.yaml)

## Why?

[GOG](https://www.gog.com/) is doing great work for gaming on Linux.

However, GOG games for Linux are executable self-installing shell scripts that install games directly into the filesystem of your machine with no isolation. This is fantastic for compatibility, but for the reasons described on the [Don't Break Debian](https://wiki.debian.org/DontBreakDebian) wiki page, this is a bad idea for system stability and long-term maintenance.

By converting these shell scripts into [Flatpaks](https://flatpak.org/), you get:

* isolated, sandboxed execution: the game runs in a containerised environment with reduced access to the rest of your system.
* package management: `flatpak list`, `flatpak remove` etc.
* cross-distro compatibility: anywhere that flatpak is available, you can install and run the game.

The gogpack launcher script also wraps the game in [gamescope](https://github.com/ValveSoftware/gamescope), which provides improved window management and compatibility with modern Linux Wayland desktops.

## How gogpack works

`gogpack` builds Flatpaks by extracting the contents of the GOG installer, parsing the provided `start.sh` script to discover compatibility quirks (like `LD_LIBRARY_PATH` / `LD_PRELOAD`), and generating a launcher script and a Flatpak manifest. Finally, these artefacts are compiled into a standalone `.flatpak` bundle using `flatpak-builder`.

Importantly, at no point does gogpack execute any game code. It is just extracting and repackaging.

## Usage

### Prerequisites

You may need to manually install the Flatpak Gamescope extension if Flatpak doesn't install it automatically as a dependency of the converted game flatpak. Note that the gamescope extension version installed needs to match the SDK version used by the Flatpak.

```bash
flatpak install org.freedesktop.Platform.VulkanLayer.gamescope
```

### Building Flatpaks

Use the `convert` command to build a Flatpak from a GOG installer script:

```bash
gogpack convert path/to/installer.sh
```

You can also include DLCs during the conversion:

```bash
gogpack convert path/to/installer.sh --dlc path/to/dlc1.sh --dlc path/to/dlc2.sh
```

You can optionally disable the Gamescope wrapper, though keeping it enabled is recommended for proper display scaling and compatibility:

```bash
gogpack convert path/to/installer.sh --gamescope=false
```

You can also specify the Flatpak SDK runtime version (see `--help` for the current default value):

```bash
gogpack convert path/to/installer.sh --runtime-version=24.08
```

#### Debugging the Build Process

If you encounter issues while building the Flatpak, you can use the `--debug` and `--preserve-workspace` flags:

- `--debug`: Enables verbose debug-level logging to help diagnose issues during the conversion.
- `--preserve-workspace`: Prevents `gogpack` from cleaning up the temporary workspace directory after building. This is useful if you want to inspect the extracted files or the generated Flatpak manifest.

```bash
gogpack --debug convert path/to/installer.sh --preserve-workspace
```

### Installing Games

Once built, you can install the generated `.flatpak` bundle:

```bash
flatpak install ./com.gog.GameName.flatpak
```

or

```bash
flatpak install --user ./com.gog.GameName.flatpak
```

### Running Games

Run the installed game via Flatpak:

```bash
flatpak run com.gog.GameName
```

The generated launcher script has some features:
- You can customize gamescope by setting the `GAMESCOPE_ARGS` environment variable (if the flatpak is built with gamescope).
- Set `FLATPAK_DEBUG=1` to enable bash execution tracing (`set -x`) in the launcher.

Example:
```bash
flatpak run --env=GAMESCOPE_ARGS="-w 1920 -h 1080 -W 1920 -H 1080" --env=FLATPAK_DEBUG=1 com.gog.GameName
```

Flatpak is integrated with desktop app launchers so it should also show up in your graphical menu of choice.

### Troubleshooting

If you encounter mouse issues—such as the cursor clicking with an offset or being unable to click certain areas of the screen—this is often caused by the game misinterpreting the display resolution. You can fix this by setting a Flatpak override for `GAMESCOPE_ARGS` to match your monitor's native resolution for both the internal game resolution (`-w`, `-h`) and the output resolution (`-W`, `-H`).

For example, to set the resolution to 4K (3840x2160):

```bash
flatpak override --user --env=GAMESCOPE_ARGS="-w 3840 -h 2160 -W 3840 -H 2160" com.gog.GameName
```

This ensures the game and Gamescope scale correctly, aligning the visual cursor with the actual clickable area. Overrides are persistent, so they will apply when you run the game via `flatpak run` or via your desktop app launcher.

To see current overrides:

```bash
flatpak override --user --show com.gog.GameName
```

To reset (remove) current overrides:

```bash
flatpak override --user --reset com.gog.GameName
```

## Game compatibility

I've only tested gogpack on a few games, so if you have had success with other games please send a PR to update this table!

| Title           | Works? | Notes                                                                                                                                |
| ---             | ---    | ---                                                                                                                                  |
| Lego Bricktales | ✅     | Use resolution override if mouse can't click near edges of the screen. e.g. `--env=GAMESCOPE_ARGS="-w 3840 -h 2160 -W 3840 -H 2160"` |
| World Of Goo    | ✅     |                                                                                                                                      |

## Prior art

* [flatpak-gog](https://github.com/kujeger/flatpak-gog) does something similar, but I wanted to try implementing my own thing.
