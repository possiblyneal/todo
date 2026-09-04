# shellcheck shell=bash
# Validation of a quadlet unit's deploy/quadlet/ pair. Sourced by scripts/package,
# which is the ship-fact dispatch and holds nothing about quadlet itself.
#
# Sourced, never executed: no shebang, no executable bit, a .sh extension so
# linters recognize it. The same rule libs/detect.sh follows, for the same
# reason.

# Every assignment of one key in a systemd unit file, one per line. systemd
# takes the last assignment of most keys, but `ImageTag` repeats where the
# option does -- `podman build` takes several `--tag` -- so all of them are
# returned and each caller decides which it means.
quadlet_values() {
  sed -n -E "s/^[[:space:]]*$2[[:space:]]*=[[:space:]]*(.*[^[:space:]])[[:space:]]*\$/\\1/p" "$1"
}

# A quadlet unit ships unit files, not an image. systemd and podman build it on
# the deploy host when the unit starts, so packaging one validates the pair and
# produces nothing. Nothing here shells out to a container runtime, which is why
# the full gate still passes on a machine with no podman installed and why
# scripts/doctor requires none.
#
# What it catches is the mismatch that would otherwise surface as a service that
# fails to start on the deploy host: a Containerfile that is not where the build
# says it is, or a container asking for an image the build never produces.
#
# quadlet_validate <dir> <label>: the directory holding the pair, and the name
# the messages report the unit under. The pair is listed with compgen rather
# than a glob so that no match is no file, whatever the caller's nullglob.
quadlet_validate() {
  local dir="$1" label="$2" file value status=0
  local -a builds=() containers=() produced=()

  mapfile -t builds < <(compgen -G "$dir/*.build" || true)
  mapfile -t containers < <(compgen -G "$dir/*.container" || true)

  if (( ${#builds[@]} == 0 )); then
    echo "$label declares ships: quadlet but $dir/ holds no .build unit." >&2
    status=1
  fi
  if (( ${#containers[@]} == 0 )); then
    echo "$label declares ships: quadlet but $dir/ holds no .container unit." >&2
    status=1
  fi
  (( status == 0 )) || return "$status"

  for file in "${builds[@]}"; do
    # The image names this build produces, plus the build unit's own file name:
    # a .container may ask for either, and quadlet resolves the second to the
    # first itself.
    mapfile -t -O "${#produced[@]}" produced < <(quadlet_values "$file" ImageTag)
    produced+=("$(basename "$file")")

    if [[ -z "$(quadlet_values "$file" ImageTag)" ]]; then
      echo "$file names no ImageTag, so nothing can depend on what it builds." >&2
      status=1
    fi

    value="$(quadlet_values "$file" File | tail -n 1)"
    if [[ -z "$value" ]]; then
      # podman reads the Containerfile out of the working directory when the
      # build names no File, so this is a legal unit rather than a broken one.
      if [[ -z "$(quadlet_values "$file" SetWorkingDirectory | tail -n 1)" ]]; then
        echo "$file names neither File nor SetWorkingDirectory, so podman has no Containerfile to build." >&2
        status=1
      fi
    elif [[ "$value" != - && "$value" != http://* && "$value" != https://* ]]; then
      # A relative File is relative to the unit file, which is what podman
      # resolves it against.
      [[ "$value" == /* ]] || value="$dir/$value"
      if [[ ! -e "$value" ]]; then
        echo "$file names File=$value, which does not exist." >&2
        status=1
      fi
    fi
  done

  for file in "${containers[@]}"; do
    value="$(quadlet_values "$file" Image | tail -n 1)"
    if [[ -z "$value" ]]; then
      echo "$file names no Image, so there is nothing for it to run." >&2
      status=1
      continue
    fi
    if ! grep -qxF -- "$value" <<< "$(printf '%s\n' "${produced[@]}")"; then
      echo "$file asks for Image=$value, which no .build unit here produces." >&2
      echo "Built here: ${produced[*]}" >&2
      status=1
    fi
  done

  (( status == 0 )) && echo "$label ships a quadlet: validated $dir/, nothing built."
  return "$status"
}
