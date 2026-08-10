#!/usr/bin/env bash
#
# Block until the PR environment is actually serving the commit under test.
#
# The deploy job only pushes a commit to the Flux repo; the rollout happens
# minutes later, and the PR hostnames keep answering from the previous release
# until it does. So this waits on two things, in order:
#
#   1. /version reports the expected release, not the last one.
#
#   2. /readyz stays green for a sustained streak. The version flips as soon as
#      the new pod starts, which is before db-migrations reseeds and before the
#      new OpenFGA store exists; /readyz goes 503 through that work, so an
#      unbroken streak is what keeps us out of the destructive window.
#
# Usage: wait-for-deployment.sh <organization-api-url> <expected-version>
#   e.g. wait-for-deployment.sh https://organization.pr349.example.com abc-br-42
#
# Tunables are env-overridable so the script can be tested against a stub.

set -uo pipefail

ORG_API="${1:?organization-api url required}"
EXPECTED_VERSION="${2:?expected version required}"

# Phase 1 budget: Flux has to notice the overlay commit, reconcile the
# HelmRelease, and pull images. 60 x 15s = 15 minutes.
VERSION_ATTEMPTS="${VERSION_ATTEMPTS:-60}"
VERSION_INTERVAL="${VERSION_INTERVAL:-15}"

# Phase 2 budget: 12 consecutive greens at 5s = 60s unbroken, and up to
# 120 samples (10 minutes) to accumulate them.
READY_STREAK_REQUIRED="${READY_STREAK_REQUIRED:-12}"
READY_INTERVAL="${READY_INTERVAL:-5}"
READY_ATTEMPTS="${READY_ATTEMPTS:-120}"

echo "Waiting for ${ORG_API} to report version ${EXPECTED_VERSION}"

deployed=""
for i in $(seq 1 "${VERSION_ATTEMPTS}"); do
  deployed=$(curl -s --max-time 5 "${ORG_API}/version" || true)

  if [ "${deployed}" = "${EXPECTED_VERSION}" ]; then
    echo "organization-api is serving ${deployed}"
    break
  fi

  if [ "${i}" -eq "${VERSION_ATTEMPTS}" ]; then
    echo "Timed out waiting for ${EXPECTED_VERSION}; still serving '${deployed:-<unreachable>}'"
    echo "The rollout never landed — check the HelmRelease and GitRepository in the PR namespace."
    exit 1
  fi

  echo "attempt ${i}/${VERSION_ATTEMPTS}: serving '${deployed:-<unreachable>}', want '${EXPECTED_VERSION}', retrying in ${VERSION_INTERVAL}s..."
  sleep "${VERSION_INTERVAL}"
done

echo "Waiting for ${ORG_API}/readyz to stay green for $((READY_STREAK_REQUIRED * READY_INTERVAL))s"

streak=0
for i in $(seq 1 "${READY_ATTEMPTS}"); do
  status=$(curl -s -o /dev/null -w "%{http_code}" --max-time 5 "${ORG_API}/readyz" || true)

  if [ "${status}" = "200" ]; then
    streak=$((streak + 1))
    echo "attempt ${i}/${READY_ATTEMPTS}: ready (${streak}/${READY_STREAK_REQUIRED})"

    if [ "${streak}" -ge "${READY_STREAK_REQUIRED}" ]; then
      echo "organization-api has been ready for ${streak} consecutive checks; environment is settled"
      exit 0
    fi
  else
    # Any blip means the rollout is still in flight; a partial streak proves
    # nothing, so start over.
    if [ "${streak}" -gt 0 ]; then
      echo "attempt ${i}/${READY_ATTEMPTS}: not ready (HTTP ${status}), streak of ${streak} reset"
    else
      echo "attempt ${i}/${READY_ATTEMPTS}: not ready (HTTP ${status})"
    fi
    streak=0
  fi

  if [ "${i}" -eq "${READY_ATTEMPTS}" ]; then
    echo "Timed out waiting for a stable /readyz (last HTTP ${status}, streak ${streak})"
    echo "Last /readyz body:"
    curl -s --max-time 5 "${ORG_API}/readyz" || true
    echo
    exit 1
  fi

  sleep "${READY_INTERVAL}"
done
