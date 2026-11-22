#!/usr/bin/env bash
# Memeriksa satu lab: kode awal harus lulus TestStep0 dan gagal di step lain,
# dan kode awal + _solusi harus lulus semua test.
# Pakai: ./verify.sh labs/09-at-least-once-delivery
set -uo pipefail
lab=${1:?pakai: ./verify.sh labs/NN-slug}; lab=${lab%/}
fail() { echo "FAIL [$lab] $1"; [ -n "${2:-}" ] && echo "$2" | tail -40; exit 1; }
[ -d "$lab/_solusi" ] || fail "tidak ada folder _solusi"

check_fmt() { local o; o=$(cd "$1" && gofmt -l . 2>&1); [ -z "$o" ] || fail "gofmt ($2)" "$o"; }

check_fmt "$lab" starter
out=$(cd "$lab" && go vet ./... 2>&1) || fail "go vet (starter)" "$out"
out=$(cd "$lab" && go test -count=1 -timeout 180s -v ./... 2>&1)
echo "$out" | grep -qE "panic:|\[build failed\]|test timed out" && fail "starter panic/build gagal/timeout" "$out"
echo "$out" | grep -q -- "--- PASS: TestStep0" || fail "TestStep0 harus lulus di kode awal" "$out"
echo "$out" | grep -q -- "--- FAIL: TestStep" || fail "kode awal harus gagal di minimal satu TestStep" "$out"

tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
cp -R "$lab/." "$tmp/" && cp -R "$tmp/_solusi/." "$tmp/" && rm -rf "$tmp/_solusi"
check_fmt "$tmp" solusi
out=$(cd "$tmp" && go vet ./... 2>&1) || fail "go vet (solusi)" "$out"
out=$(cd "$tmp" && go test -race -count=1 -timeout 300s ./... 2>&1) || fail "solusi tidak lulus" "$out"
echo "OK   [$lab]"
