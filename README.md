# Concurrent Program Rules

A collection of design rules for building correct concurrent systems, drawn from real production failures rather than language-specific patterns.

This repository accompanies a blog series called **Rules for Designing Concurrent Systems**, where each lesson captures a principle that explains why systems stall, saturate, or quietly stop making progress without crashing.

## What This Is

- Design-level rules, not API tutorials
- Focused on backpressure, ownership, signaling, coordination, and failure modes
- Based on scenarios that look healthy but are fundamentally broken

Each rule follows the same structure:
- A realistic failure scenario
- Common but incorrect explanations
- The underlying design mistake
- The mindset shift that fixes it

## Current Rules

1. **Concurrency isn’t about “making things async.”**  
   It’s about controlling backpressure and ownership.

2. **Signals are not queues.**  
   If you treat them the same, your system will eventually stop.

More rules will be added over time.

## Who This Is For

Engineers who:
- Have shipped concurrent systems
- Have been bitten by “nothing is wrong but nothing works”
- Want mental models, not just patterns

## Status

Work in progress. Rules are added as articles are published.

Contributions and discussions are welcome.
