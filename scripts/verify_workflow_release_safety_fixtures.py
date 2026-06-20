#!/usr/bin/env python3
"""Self-test fixtures for workflow release safety verifier."""

FIXTURES = [
    # 1: .yaml extension is scanned
    ("fixture01-yaml-ext.yaml", """name: fixture-yaml-ext
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo build
""", []),
    # 2: release command in build job with block-form contents: read
    ("fixture02-release-read.yml", """name: fixture-release-read
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - run: gh release create "v1.0.0" dist/*.bin
  publish:
    runs-on: ubuntu-latest
    permissions: { contents: write }
    needs: build
    steps:
      - run: echo publish
""", ["BUILD-RELEASE-CREATE"]),
    # 3: gh release upload in build job
    ("fixture03-upload.yml", """name: fixture-upload
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: gh release upload "v1.0.0" dist/*.bin
  publish:
    runs-on: ubuntu-latest
    permissions: { contents: write }
    needs: build
    steps:
      - run: echo publish
""", ["BUILD-RELEASE-UPLOAD"]),
    # 4: gh release edit in build job
    ("fixture04-edit.yml", """name: fixture-edit
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: gh release edit "v1.0.0" --draft=false
  publish:
    runs-on: ubuntu-latest
    permissions: { contents: write }
    needs: build
    steps:
      - run: echo publish
""", ["BUILD-RELEASE-EDIT"]),
    # 5: publish job without needs
    ("fixture05-no-needs.yml", """name: fixture-no-needs
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: make build
  publish:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - run: gh release create "v1.0.0"
""", ["PUBLISH-NO-NEEDS"]),
    # 6: workflow-level write with publish jobs (non-release)
    ("fixture06-wf-write.yml", """name: fixture-workflow-write
on:
  push:
    tags:
      - "v*"
permissions:
  contents: write
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: make build
  publish:
    runs-on: ubuntu-latest
    permissions: { contents: write }
    needs: build
    steps:
      - run: gh release create "v1.0.0"
""", []),
    # 7: two publish jobs fail
    ("fixture07-two-publish.yml", """name: fixture-two-publish
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: make build
  publish-opkg:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    needs: build
    steps:
      - run: gh release create "v1.0.0"
  publish-deb:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    needs: build
    steps:
      - run: gh release create "v1.0.0"
""", ["MULTIPLE-PUBLISH-JOBS"]),
    # 8: release-opkg.yml with no tags
    ("release-opkg.yml", """name: release-opkg
on: push
jobs:
  build-opkg:
    runs-on: ubuntu-latest
    steps:
      - run: make build
  publish-opkg-release:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    needs: build-opkg
    steps:
      - run: gh release create "v1.0.0"
""", ["MISSING-TAG-PATTERN"]),
    # 9: tovarisch-deb-release.yml with no tags
    ("tovarisch-deb-release.yml", """name: tovarisch-deb-release
on: push
jobs:
  build-deb:
    runs-on: ubuntu-latest
    steps:
      - run: make build
  publish-deb-release:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    needs: build-deb
    steps:
      - run: gh release create "v1.0.0"
""", ["MISSING-TAG-PATTERN"]),
    # 10: legacy v* in test-release-opkg.yml
    ("test-release-opkg.yml", """name: fixture-legacy-tag
on:
  push:
    tags:
      - "v*"
jobs:
  build-opkg:
    runs-on: ubuntu-latest
    steps:
      - run: make build
  publish-opkg-release:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    needs: build-opkg
    steps:
      - run: gh release create "v1.0.0"
""", ["LEGACY-TAG-PATTERN"]),
    # 11: old opkg-rc pattern in test-tovarisch-deb-release.yml
    ("test-tovarisch-deb-release.yml", """name: fixture-old-trigger
on:
  push:
    tags:
      - "v0.1.1-opkg-rc*"
jobs:
  build-deb:
    runs-on: ubuntu-latest
    steps:
      - run: make build
  publish-release:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    needs: build-deb
    steps:
      - run: gh release create "v1.0.0"
""", ["LEGACY-TAG-PATTERN"]),
    # 12: valid uvb76-opkg release workflow
    ("fixture12-valid-opkg.yml", """name: fixture-valid-opkg
on:
  push:
    tags:
      - "uvb76-opkg-v*"
permissions:
  contents: read
jobs:
  build-opkg:
    runs-on: ubuntu-latest
    steps:
      - run: make build
      - uses: actions/upload-artifact@v4
        with:
          name: packages
          path: dist/*.ipk
  publish-opkg-release:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    needs: build-opkg
    steps:
      - run: gh release create "${GITHUB_REF_NAME}"
""", []),
    # 13: release workflow with workflow-level write fails
    # Uses tovarisch-release.yml (in RELEASE_WORKFLOWS) with workflow-level write
    ("tovarisch-release.yml", """name: fixture-release-wf-write
on:
  push:
    tags:
      - "v*"
permissions:
  contents: write
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: make build
  publish:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    needs: build
    steps:
      - run: gh release create "${GITHUB_REF_NAME}"
""", ["WORKFLOW-LEVEL-WRITE", "LEGACY-TAG-PATTERN"]),
    # 14: non-release with wrong tag - no violation
    ("fixture14-wrong-tag.yml", """name: fixture-wrong-pattern
on:
  push:
    tags:
      - "wrong-pattern-v*"
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: make build
  publish:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    needs: build
    steps:
      - run: gh release create "v1.0.0"
""", []),
]
