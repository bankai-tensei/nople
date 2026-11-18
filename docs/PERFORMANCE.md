# Performance numbers

Where the numbers in the README and elsewhere come from. The measures are in the repository, so they can be taken again the same way. When you quote a number, say on what machine and with what cache it was measured.

[日本語](PERFORMANCE.ja.md)

## Machine

Measured on 2026-11-18.

| | |
|---|---|
| CPU | Intel Core i7-12700H (14 cores, 20 threads) |
| Memory | 16 GB |
| Disk | NVMe SSD (Phison 1 TB) |
| GPU | NVIDIA GeForce RTX 3050 Ti Laptop, Intel Iris Xe |
| OS | Windows 11 Pro (10.0.26300); Linux on WSL 2 |
| Nople | the work after `7a6dde5`, built with `-trimpath -ldflags "-H windowsgui -s -w"` |

## Start

From the creation of the process, as the OS recorded it, to the first frame drawn, and to the frame that shows what was opened (a folder, a file). Medians of 20 starts, leaving out the first start after building.

| Opened | First frame | What was opened |
|---|---|---|
| Nothing (`-fresh`) | 0.10 s (0.10–0.14) | 0.13 s (0.12–0.18) |
| A folder (this repository) | 0.10 s (0.09–0.11) | 0.12 s (0.12–0.14) |
| One Go file | 0.11 s (0.10–0.13) | 0.14 s (0.14–0.17) |

- The first start after building drew its first frame at 0.22 s. A start fully cold, as just after a reboot, was not measured.
- "What was opened" is the frame just after reading the folder and coloring the file have started in the background; the state of git comes after.

### Idle

With a folder or a file open and nothing done, the CPU time over each of three stretches of 5 seconds was 0 ms. The working set is about 250 MB, of which the heap of Go is about 95 MB.

### How

Set `NOPLE_STARTUP_LOG` to a file, and Nople appends its measures to it and quits.

```powershell
$env:NOPLE_STARTUP_LOG = "$env:TEMP\startup.txt"
1..21 | ForEach-Object { Start-Process nople -ArgumentList '-fresh','C:\path\to\folder' -Wait }
Get-Content $env:TEMP\startup.txt
```

Elsewhere than on Windows the start is when the package of Nople initialized, not when the OS created the process, so loading the executable is not counted.

## Large files

Logs of lines of about 80 bytes, written to disk and then opened. Medians of 5 opens of each size.

- **Shows**: until `Open` returns, when the first 1 MB is indexed and the editor can show the start.
- **Whole index**: until the rest of the lines are indexed. The file shows, scrolls and edits meanwhile.
- **Index memory**: what the heap of Go grew by for the index.

### Windows, the file in the cache of the OS

Windows does not let the cache of a file be dropped without administrator rights; the numbers are of files just written, in the cache.

| Size | Lines | Shows | Whole index | Index memory |
|---|---|---|---|---|
| 100 MB | 1.28 million | under 2 ms | 0.03 s | 1.0 MB |
| 1 GB | 13.1 million | under 2 ms | 0.30 s | 4.4 MB |
| 10 GB | 131 million | under 2 ms | 8.3 s | 36 MB |

- The clock of Go on Windows is coarse and reads less than 1 ms as 0; the medians of "shows" were 0 to 1.8 ms.
- The first open of each size took 14–24 ms to show.
- 10 GB does not all fit in 16 GB of memory, and is read in part from the disk.

### Linux on WSL 2, with the cache dropped and in it

The files were on the ext4 inside WSL. "Dropped" ran `echo 3 > /proc/sys/vm/drop_caches` before each open; what Windows caches of the virtual disk of WSL stays.

| Size | Cache | Shows | Whole index |
|---|---|---|---|
| 100 MB | dropped | 5.9 ms | 0.07 s |
| 1 GB | dropped | 6.1 ms | 0.65 s |
| 10 GB | dropped | 6.9 ms | 19.9 s |
| 100 MB | in it | 0.9 ms | 0.02 s |
| 1 GB | in it | 0.5 ms | 0.23 s |

### How

```bash
NOPLE_MEASURE=100,1024,10240 NOPLE_MEASURE_DIR=/path/to/dir NOPLE_MEASURE_RUNS=5 go test -run TestMeasure -v -timeout 60m ./internal/bigfile
```

To drop the cache on Linux before each open, add `NOPLE_MEASURE_DROP='sync; echo 3 > /proc/sys/vm/drop_caches'` and run as root. See [internal/bigfile/measure_test.go](../internal/bigfile/measure_test.go).

## Against other editors

Nople was measured against VS Code and Zed on the same machine (2026-11-18, the machine above): each editor was started with a large log as its argument, and timed until the contents of the file showed in its window, with the highest memory meanwhile (the working sets of all the editor's processes, summed).

| | Nople (23a4b18) | VS Code 1.133.0 | Zed 1.27.0 |
|---|---|---|---|
| 1 GB: until the contents show | 0.31 s | 8.0 s | 2.1 s |
| 1 GB: memory, highest | 271 MB | 3.0 GB | 2.7 GB |
| 10 GB: until the contents show | 0.35 s | 117 s | does not open ("File is too large to load") |
| 10 GB: memory, highest | 392 MB | 5.8 GB | — |

- **Timing**: the window was captured every 0.03 to 0.05 s, and the time taken is the first capture that matches the last one. Nothing was typed or clicked. The animation of a new window and the delay of the compositor are included, so these run longer than the start measured above. All three were measured the same way.
- **Runs**: 1 GB is the median of 3 starts, after a first start left out; so is Nople at 10 GB. VS Code and Zed at 10 GB were started once.
- **Files**: logs of lines of about 80 bytes, in the cache of the OS.
- **Settings**: each editor ran with its defaults.
  - Nople was started as `nople -fresh <file>`, with only its language and theme set.
  - VS Code was started with empty user and extensions folders, with only the prompt before opening a large file turned off. On a large file, VS Code itself turns off highlighting, folding and the like.
  - Zed was started with an empty user folder.
- **Nople's index**: Nople goes on indexing the file in the background after it shows. At 10 GB, the line numbers stopped growing wider, shifting the view, after about 4.6 s.
- **Script**: the script and the raw results are in [tools/compare/](../tools/compare/) (`measure.py` and `results-2026-11-18.jsonl`). It runs on Windows and needs Python 3, Pillow and numpy; its use is at the top of `measure.py`.

## What the numbers say, and what they do not

- "A log of 1 GB opens at once" and "10 GB opens too" hold: the file shows within 7 ms at every size, with the cache dropped; the whole index goes as fast as the disk reads.
- "Starts in 0.1 s" is on this machine, warm. Slower machines and a start just after a reboot were not measured.
- The comparison with other editors is on the one machine above, with the files in the cache and default settings; VS Code and Zed at 10 GB were measured once. Say so alongside any comparison.
