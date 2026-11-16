
# generated with CMTools 0.0.0 f50285c

#
# Simple Makefile for Golang based Projects built under POSIX.
#
PROJECT = rptop

GIT_GROUP = rsdoiel

PROGRAMS = rptop

VERBS = temp cpu mem fan volts disk net throttle record list

# The Hailo temperature needs cgo and libhailort (DR-0001). Build with the
# hailo tag when the HailoRT header is installed; without it the element
# reports absent (exit 66). Override with GO_TAGS= to force a plain build.
GO_TAGS = $(shell test -f /usr/include/hailo/hailort.h && echo "-tags hailo")

RELEASE_DATE = $(shell date +%Y-%m-%d)

RELEASE_HASH=$(shell git log --pretty=format:'%h' -n 1)

# The section 1 pages are rptop and one page per verb, taken from PROGRAMS and VERBS
# and not from a glob over the .1.md files: a missing source file would otherwise drop
# its page from `make man` and `make install` and the build would still succeed.
# internal/cli/makefile_test.go checks this and that VERBS matches the help pages.
MAN_PAGES_1 = $(addsuffix .1,$(PROGRAMS) $(addprefix $(PROGRAMS)-,$(VERBS)))

MAN_PAGES_3 = $(patsubst %.3.md,%.3,$(wildcard *.3.md))

MAN_PAGES_7 = $(patsubst %.7.md,%.7,$(wildcard *.7.md))

HTML_PAGES = $(shell find . -type f -not -path './bin/*' -not -path './.git/*' | grep -E '\.html$$')

DOCS = $(wildcard *.?.md)

# version.go is left out: it has a rule (cmt, under generate), and listing it
# here would make every build run cmt through that rule.
PACKAGE = $(shell find . -name '*.go' -not -name '*_test.go' -not -name version.go -not -path './bin/*' -not -path './dist/*')

VERSION = $(shell grep '"version":' codemeta.json | cut -d\"  -f 4)

BRANCH = $(shell git branch --show-current)

OS = $(shell uname)

#PREFIX = /usr/local/bin
PREFIX = $(HOME)

ifneq ($(prefix),)
	PREFIX = $(prefix)
endif

EXT =
ifeq ($(OS), Windows)
	EXT = .exe
endif

# build compiles the programs and regenerates the help text and man pages. It
# does not run cmt: version.go, CITATION.cff, about.md and installer.sh are
# generated from codemeta.json by `make generate` (rptop is Linux arm64 only,
# so there is no installer.ps1), so the version and release
# hash change only when that is asked for (the release target does).
build: $(PROGRAMS) verb-help man

# generate runs cmt over codemeta.json. Run it when the version, the release
# notes or the metadata change, and before a release; a plain `make` leaves
# the committed version.go, about.md and friends alone.
generate: version.go CITATION.cff about.md installer.sh

version.go: .FORCE
	cmt codemeta.json version.go

hash: .FORCE
	git log --pretty=format:'%h' -n 1

man: $(MAN_PAGES_1) # $(MAN_PAGES_3) $(MAN_PAGES_7)

$(MAN_PAGES_1): .FORCE
	mkdir -p man/man1
	pandoc $@.md --from markdown --to man -s >man/man1/$@

$(MAN_PAGES_3): .FORCE
	mkdir -p man/man3
	pandoc $@.md --from markdown --to man -s >man/man3/$@

$(MAN_PAGES_7): .FORCE
	mkdir -p man/man7
	pandoc $@.md --from markdown --to man -s >man/man7/$@

$(PROGRAMS): $(PACKAGE)
	@mkdir -p bin
	go build $(GO_TAGS) -o "bin/$@$(EXT)" cmd/$@/*.go
	@./bin/$@ -help >$@.1.md

# verb-help regenerates each verb's own rptop-VERB.1.md man-page source
# (rptop temp -help, rptop record -help, etc.) after bin/rptop is built. The list
# is VERBS, and MAN_PAGES_1 is built from the same list, so a verb's page is made
# before `make man` asks for it. internal/cli/makefile_test.go keeps VERBS and the
# verbs in step.
verb-help: $(PROGRAMS)
	@if [ -n "$(VERBS)" ]; then for verb in $(VERBS); do \
		./bin/rptop $$verb -help >rptop-$$verb.1.md; \
	done; fi

$(MAN_PAGES): .FORCE
	mkdir -p man/man1
	pandoc $@.md --from markdown --to man -s >man/man1/$@

CITATION.cff: .FORCE
	cmt codemeta.json CITATION.cff

about.md: codemeta.json $(PROGRAMS)
	cmt codemeta.json about.md

installer.sh: .FORCE
	cmt codemeta.json installer.sh


test: $(PACKAGE)
	go test $(GO_TAGS) ./...

website: clean-website .FORCE
	make -f website.mak

status:
	git status

save:
	@if [ "$(msg)" != "" ]; then git commit -am "$(msg)"; else git commit -am "Quick Save"; fi
	git push origin $(BRANCH)

refresh:
	git fetch origin
	git pull origin $(BRANCH)

publish: build website .FORCE
	./publish.bash

clean:
	@if [ -f version.go ]; then rm version.go; fi
	@if [ -d bin ]; then rm -fR bin; fi
	@if [ -d dist ]; then rm -fR dist; fi
	@if [ -d man ]; then rm -fR man; fi
	@if [ -d testout ]; then rm -fR testout; fi

clean-website:
	@for FNAME in $(HTML_PAGES); do if [ -f "$${FNAME}" ]; then rm "$${FNAME}"; fi; done

install: build
	@echo "Installing programs in $(PREFIX)/bin"
	@for FNAME in $(PROGRAMS); do if [ -f "./bin/$${FNAME}$(EXT)" ]; then mv -v "./bin/$${FNAME}$(EXT)" "$(PREFIX)/bin/$${FNAME}$(EXT)"; fi; done
	@echo ""
	@echo "Make sure $(PREFIX)/bin is in your PATH"
	@echo "Installing man page in $(PREFIX)/man"
	@mkdir -p $(PREFIX)/man/man1
	@for FNAME in $(MAN_PAGES_1); do if [ -f "./man/man1/$${FNAME}" ]; then cp -v "./man/man1/$${FNAME}" "$(PREFIX)/man/man1/$${FNAME}"; fi; done
	@mkdir -p $(PREFIX)/man/man3
	@for FNAME in $(MAN_PAGES_3); do if [ -f "./man/man3/$${FNAME}" ]; then cp -v "./man/man3/$${FNAME}" "$(PREFIX)/man/man3/$${FNAME}"; fi; done
	@mkdir -p $(PREFIX)/man/man7
	@for FNAME in $(MAN_PAGES_7); do if [ -f "./man/man7/$${FNAME}" ]; then cp -v "./man/man7/$${FNAME}" "$(PREFIX)/man/man7/$${FNAME}"; fi; done
	@echo ""
	@echo "Make sure $(PREFIX)/man is in your MANPATH"

uninstall: .FORCE
	@echo "Removing programs in $(PREFIX)/bin"
	@for FNAME in $(PROGRAMS); do if [ -f "$(PREFIX)/bin/$${FNAME}$(EXT)" ]; then rm -v "$(PREFIX)/bin/$${FNAME}$(EXT)"; fi; done
	@echo "Removing man pages in $(PREFIX)/man"
	@for FNAME in $(MAN_PAGES_1); do if [ -f "$(PREFIX)/man/man1/$${FNAME}" ]; then rm -v "$(PREFIX)/man/man1/$${FNAME}"; fi; done
	@for FNAME in $(MAN_PAGES_3); do if [ -f "$(PREFIX)/man/man3/$${FNAME}" ]; then rm -v "$(PREFIX)/man/man3/$${FNAME}"; fi; done
	@for FNAME in $(MAN_PAGES_7); do if [ -f "$(PREFIX)/man/man7/$${FNAME}" ]; then rm -v "$(PREFIX)/man/man7/$${FNAME}"; fi; done

setup_dist: .FORCE
	@mkdir -p dist
	@rm -fR dist/

dist/Linux-aarch64: $(PROGRAMS)
	@mkdir -p dist/bin
	@for FNAME in $(PROGRAMS); do env  GOOS=linux GOARCH=arm64 go build $(GO_TAGS) -o "dist/bin/$${FNAME}" cmd/$${FNAME}/*.go; done
	@cd dist && zip -r $(PROJECT)-v$(VERSION)-Linux-aarch64.zip LICENSE codemeta.json CITATION.cff *.md bin/* man/* $(DOCS)
	@rm -fR dist/bin

distribute_docs:
	@mkdir -p dist/
	@cp -v codemeta.json dist/
	@cp -v CITATION.cff dist/
	@cp -v README.md dist/
	@cp -v LICENSE dist/
	@cp -v INSTALL.md dist/
	@cp -v CHANGES.md dist/
	@cp -vR man dist/
	@for DNAME in $(DOCS); do cp -vR $$DNAME dist/; done

# check-go-version enforces the Go version policy: the minimum Go is the highest
# version the release was built with, and every file that states it agrees.
# It writes nothing. It fails when
#   - the go line of go.mod is lower than the Go in use (go env GOVERSION),
#   - the "Go >= " line of codemeta.json is not the go line of go.mod,
#   - README.md or INSTALL.md does not say "Go >= " with that version.
# A go line without a patch ("1.27") is read as 1.27.0. check-release runs it.
check-go-version: .FORCE
	@FAIL=0; \
	bad() { echo "check-go-version: $$1"; FAIL=1; }; \
	full() { case "$$1" in *.*.*) echo "$$1";; *) echo "$$1.0";; esac; }; \
	TC="$$(go env GOVERSION | sed 's/^go//')"; \
	case "$$TC" in [0-9]*.[0-9]*) ;; *) TC="";; esac; \
	case "$$TC" in *[!0-9.]*) TC="";; esac; \
	if [ -z "$$TC" ]; then bad "cannot read the Go version from go env GOVERSION (a release is built with a tagged Go)"; \
	else \
		TC="$$(full "$$TC")"; \
		GM="$$(full "$$(sed -n 's/^go \([0-9.]*\).*/\1/p' go.mod | head -1)")"; \
		LOW="$$(printf '%s\n%s\n' "$$GM" "$$TC" | sort -V | head -1)"; \
		[ "$$GM" = "$$TC" ] || [ "$$LOW" != "$$GM" ] || bad "go.mod says go $$GM, the Go in use is $$TC: the minimum is the highest version the release is built with"; \
		CM="$$(jq -r '[.softwareRequirements[]? | select(startswith("Go >= "))][0] // empty | sub("Go >= "; "")' codemeta.json)"; \
		[ -n "$$CM" ] || bad "codemeta.json has no \"Go >= \" line in softwareRequirements"; \
		[ -z "$$CM" ] || [ "$$(full "$$CM")" = "$$GM" ] || bad "codemeta.json says Go >= $$CM, go.mod says go $$GM"; \
		for F in README.md INSTALL.md; do \
			grep -q "Go >= $$GM" "$$F" || bad "$$F does not say \"Go >= $$GM\""; \
		done; \
	fi; \
	if [ $$FAIL -ne 0 ]; then exit 1; fi; \
	echo "check-go-version: Go $$GM is consistent"

# check-requirements keeps codemeta.json's softwareRequirements and go.mod in step, in
# both directions. A softwareRequirements entry that is a Go module ("charm.land/x/v2
# >= 2.1.0", the first path element having a dot) must be a direct requirement of go.mod
# at that version, and every direct requirement of go.mod must be listed. Entries that are
# not modules ("Go >= 1.27.3") are left alone. It writes nothing. check-release runs it.
check-requirements: .FORCE
	@LISTED="$$(jq -r '.softwareRequirements[]? | select(test("^[A-Za-z0-9-]+(\\.[A-Za-z0-9-]+)+/")) | gsub(" >= "; " ")' codemeta.json)"; \
	WANT="$$(go list -m -f '{{if and (not .Indirect) (not .Main)}}{{.Path}} {{.Version}}{{end}}' all)"; \
	PROBS="$$( { printf '%s\n' "$$WANT" | sed 's/^/W /'; printf '%s\n' "$$LISTED" | sed 's/^/L /'; } | awk ' \
		$$1 == "W" && NF >= 3 { want[$$2] = $$3; next } \
		$$1 == "L" && NF >= 3 { listed[$$2] = $$3 } \
		END { \
			for (m in listed) { \
				if (!(m in want)) { print "softwareRequirements names " m ", which go.mod does not require directly"; continue } \
				w = want[m]; sub(/^v/, "", w); \
				if (w != listed[m]) print "softwareRequirements says " m " >= " listed[m] ", go.mod requires v" w \
			} \
			for (m in want) if (!(m in listed)) print "go.mod requires " m " " want[m] ", softwareRequirements does not list it" \
		}' | sort )"; \
	if [ -n "$$PROBS" ]; then printf '%s\n' "$$PROBS" | sed 's/^/check-requirements: /'; exit 1; fi; \
	echo "check-requirements: softwareRequirements and go.mod are consistent"

# check-release reads the files a release is cut from and fails when they
# disagree; it writes nothing. Run it before `make release` (the release target
# runs it too, after generate). It fails when
#   - version.go, CITATION.cff or an installer carries a version other than codemeta.json's,
#     or CITATION.cff's date-released is not datePublished (run `make generate`),
#   - datePublished is not today's date, the release day,
#   - v$(VERSION) is already tagged here or on origin (codemeta.json was not bumped),
#   - releaseNotes are the same as the last tag's (not written for this release),
#   - softwareRequirements and go.mod disagree about the Go modules (check-requirements),
#   - the Go version policy fails (check-go-version).
check-release: .FORCE
	@FAIL=0; V="$(VERSION)"; TODAY="$$(date +%Y-%m-%d)"; \
	bad() { echo "check-release: $$1"; FAIL=1; }; \
	[ -n "$$V" ] || bad "codemeta.json has no version"; \
	GOV="$$(sed -n 's/^ *Version = "\(.*\)".*/\1/p' version.go | head -1)"; \
	[ "$$GOV" = "$$V" ] || bad "version.go has '$$GOV', codemeta.json has '$$V' (run make generate)"; \
	CFF="$$(sed -n 's/^version: *//p' CITATION.cff | head -1)"; \
	[ "$$CFF" = "$$V" ] || bad "CITATION.cff has '$$CFF', codemeta.json has '$$V' (run make generate)"; \
	DR="$$(sed -n 's/^date-released: *//p' CITATION.cff | head -1)"; \
	PUB="$$(jq -r '.datePublished // empty' codemeta.json)"; \
	[ "$$DR" = "$$PUB" ] || bad "CITATION.cff date-released is '$$DR', datePublished is '$$PUB' (run make generate)"; \
	[ "$$PUB" = "$$TODAY" ] || bad "datePublished is '$$PUB', today is '$$TODAY': set datePublished and dateModified to the release day"; \
	for F in installer.sh; do \
		[ -f "$$F" ] || continue; \
		IV="$$(grep -m1 'VERSION *= *"' "$$F" | sed 's/.*VERSION *= *"\([^"]*\)".*/\1/')"; \
		[ "$$IV" = "$$V" ] || bad "$$F has '$$IV', codemeta.json has '$$V' (run make generate)"; \
	done; \
	if git rev-parse -q --verify "refs/tags/v$$V" >/dev/null; then bad "v$$V is already tagged here: codemeta.json was not bumped"; fi; \
	if git ls-remote --exit-code --tags origin "refs/tags/v$$V" >/dev/null 2>&1; then bad "v$$V is already tagged on origin: codemeta.json was not bumped"; fi; \
	NEW="$$(jq -r '.releaseNotes // empty' codemeta.json)"; \
	[ -n "$$NEW" ] || bad "codemeta.json has no releaseNotes"; \
	LAST="$$(git tag --list 'v*' --sort=-v:refname | head -1)"; \
	if [ -n "$$LAST" ] && [ "$$LAST" != "v$$V" ]; then \
		OLD="$$(git show "$$LAST:codemeta.json" 2>/dev/null | jq -r '.releaseNotes // empty')"; \
		[ "$$NEW" != "$$OLD" ] || bad "releaseNotes are the same as $$LAST's: write them for v$$V"; \
	fi; \
	$(MAKE) -s check-requirements || FAIL=1; \
	$(MAKE) -s check-go-version || FAIL=1; \
	if [ $$FAIL -ne 0 ]; then echo "check-release: v$$V is NOT ready to release"; exit 1; fi; \
	echo "check-release: v$$V is consistent"

# release does not commit or push: release.bash is the only place that does.
release: generate check-release build setup_dist distribute_docs dist/Linux-aarch64
	@printf "\nready to run\n\n\trelease.bash\n\n"


.FORCE:
