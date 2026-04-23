#!/bin/bash
cd notes
git add notes.md
echo "Add exit code: $?"
git commit -m "Test"
echo "Commit exit code: $?"
