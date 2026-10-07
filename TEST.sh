#!/bin/bash

#uv run main.py pipeline \
#    --platform=rhoai-3.6-ea.2 \
#    --phase=discover-components \
#    --harness=codex \
#    --model=gpt-5.6-sol \
#    --max-concurrent=20 \
#    --force

uv run main.py pipeline \
    --platform=rhoai-3.6-ea.2 \
    --phase=static-analysis \
    --phase=generate-architecture \
    --component=rhods-operator \
    --harness=codex \
    --model=gpt-5.6-sol \
    --max-concurrent=20 \
    --force
