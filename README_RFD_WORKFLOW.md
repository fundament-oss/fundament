
  RFD Pull Request Workflow

  This document describes the complete workflow for creating, reviewing, and publishing RFDs (Requests for Discussion) using GitHub pull requests.

  RFD States

  RFDs progress through different states during their lifecycle:

  | State         | Description                                    | Where it lives              |
  |---------------|------------------------------------------------|-----------------------------|
  | prediscussion | Initial state, before formal discussion begins | Feature branch              |
  | ideation      | Early stage, sharing initial thoughts          | Feature branch with PR      |
  | discussion    | Actively being discussed                       | Feature branch with open PR |
  | published     | Approved and merged                            | Default branch (rfd)        |
  | committed     | Implementation committed/delivered             | Default branch (rfd)        |
  | abandoned     | No longer being pursued                        | Any branch                  |

  Repository Structure

  your-repo/
  ├── rfd/
  │   ├── 0001/
  │   │   └── README.adoc
  │   ├── 0002/
  │   │   └── README.adoc
  │   └── 0003/
  │       ├── README.adoc
  │       └── figures/
  │           └── diagram.png

  Complete Workflow

  1. Create a New RFD

  Step 1.1: Determine the RFD Number

  - Check existing RFDs to find the next available number
  - RFD numbers are sequential: 0001, 0002, 0003, etc.

  Step 1.2: Create a Feature Branch

  # Branch name format: {rfd-number}
  git checkout -b 0042

  Important: The branch name MUST be just the 4-digit RFD number (e.g., 0042)

  Step 1.3: Create the RFD Directory and File

  mkdir -p rfd/0042

  Create rfd/0042/README.adoc with this template:

  :showtitle:
  :toc: left
  :numbered:
  :icons: font
  :state: prediscussion
  :discussion:
  :authors: Your Name <your.email@example.com>
  :labels: category1, category2
  :revremark: State: {state}

  = RFD 42 Your RFD Title Here
  {authors}

  == Introduction

  Brief introduction of what this RFD proposes.

  == Background

  Why is this needed? What problem does it solve?

  == Proposal

  ### Overview

  Main proposal details.

  ### Implementation Details

  Technical details of the proposal.

  ### Alternatives Considered

  What other approaches were considered and why were they rejected?

  ## Open Questions

  * Question 1?
  * Question 2?

  ## References

  * https://example.com/relevant-doc[Relevant Documentation]

  Step 1.4: Commit and Push

  git add rfd/0042/
  git commit -m "RFD 42: Initial draft of [Your Title]"
  git push -u origin 0042

  2. Move to Discussion

  Step 2.1: Update the RFD State

  Edit rfd/0042/README.adoc and change:
  :state: prediscussion
  to:
  :state: discussion

  Step 2.2: Commit and Push

  git add rfd/0042/README.adoc
  git commit -m "Move RFD 42 to discussion state"
  git push

  Step 2.3: Create Pull Request

  - Go to GitHub and create a PR from branch 0042 to your default branch (rfd)
  - The processor will automatically:
    - Detect the PR
    - Update the :discussion: field in your RFD with the PR URL
    - Update the PR title to match the RFD title
    - Add appropriate labels to the PR

  Note: If you have the CreatePullRequest action enabled in your processor config, it will automatically create the PR for you when the state changes to
  discussion.

  3. Discuss and Iterate

  Step 3.1: Make Changes Based on Feedback

  # Edit your RFD file
  vim rfd/0042/README.adoc

  # Commit changes
  git add rfd/0042/README.adoc
  git commit -m "Address feedback: clarify implementation details"
  git push

  Step 3.2: Processor Actions (Automatic)

  Each time you push, if configured, the processor will:
  - ✅ Update the discussion URL if it changed
  - ✅ Generate a new PDF version
  - ✅ Update the search index
  - ✅ Copy images to storage
  - ✅ Ensure the RFD state is valid (discussion, ideation, published, committed, or abandoned)

  Step 3.3: Valid States for Open PRs

  While your PR is open, the RFD can be in these states:
  - discussion - Default state for active discussion
  - ideation - Early exploration phase
  - published - Being published or updating a published RFD
  - committed - Being committed or updating a committed RFD
  - abandoned - Marking as abandoned

  Important: If the processor finds an invalid state, it will automatically change it to discussion.

  4. Approval and Merge

  Step 4.1: Get Approval

  - Request reviews from stakeholders
  - Address all feedback
  - Get required approvals per your team's process

  Step 4.2: Update State Before Merging

  Edit rfd/0042/README.adoc and change:
  :state: discussion
  to:
  :state: published

  Commit and push:
  git add rfd/0042/README.adoc
  git commit -m "Mark RFD 42 as published"
  git push

  Step 4.3: Merge the Pull Request

  - Merge the PR via GitHub UI (or command line)
  - Delete the feature branch 0042 after merging

  Step 4.4: Processor Validation (Automatic)

  After merging to the default branch, the processor will:
  - ✅ Verify the RFD state is published, committed, or abandoned
  - ⚠️ Warn if it's in any other state
  - ✅ Update all configured integrations (search, PDFs, etc.)

  5. Update a Published RFD

  Step 5.1: Create Update Branch

  # Use the same branch naming: {rfd-number}
  git checkout -b 0042

  Step 5.2: Make Changes

  vim rfd/0042/README.adoc
  git add rfd/0042/README.adoc
  git commit -m "Update RFD 42: Add new implementation details"
  git push -u origin 0042

  Step 5.3: Create Pull Request

  - Create PR from 0042 to default branch
  - The state should remain published since you're updating an already-published RFD
  - Processor will update the discussion URL automatically

  Step 5.4: Review and Merge

  - Get approval
  - Merge to default branch
  - The RFD remains in published state

  State Transition Rules

  On Feature Branches with PRs

  ✅ Valid states: discussion, ideation, published, committed, abandoned❌ Invalid states: prediscussion (will auto-change to discussion)

  On Default Branch (rfd)

  ✅ Valid states: published, committed, abandoned⚠️ Invalid states: discussion, ideation, prediscussion (will trigger warning)

  Processor Configuration

  To enable automatic PR management, ensure your config.toml has these actions enabled:

  actions = [
    "CreatePullRequest",           # Auto-create PRs for discussion-state RFDs
    "UpdatePullRequest",           # Keep PR title/labels in sync with RFD
    "UpdateDiscussionUrl",         # Auto-update :discussion: field
    "EnsureRfdWithPullRequestIsInValidState",  # Enforce valid states on PRs
    "EnsureRfdOnDefaultIsInValidState",        # Enforce valid states on default
    "UpdateSearch",                # Update search index
    "UpdatePdfs",                  # Generate PDFs
    "CopyImagesToStorage",         # Copy images to storage
  ]

  Common Scenarios

  Scenario 1: Abandon an RFD in Discussion

  # On your feature branch
  vim rfd/0042/README.adoc
  # Change :state: to abandoned

  git add rfd/0042/README.adoc
  git commit -m "Abandon RFD 42"
  git push

  # Close the PR without merging

  Scenario 2: Multiple People Working on the Same RFD

  # Person A creates the branch and PR
  git checkout -b 0042
  # ... create RFD, push, create PR

  # Person B wants to contribute
  git fetch origin
  git checkout 0042
  # ... make changes
  git push origin 0042

  Note: The processor only updates PRs when there's exactly ONE open PR for the branch.

  Scenario 3: RFD Committed/Implemented

  After the RFD is implemented:
  git checkout 0042  # Or create new branch from default
  vim rfd/0042/README.adoc
  # Change :state: to committed

  git add rfd/0042/README.adoc
  git commit -m "Mark RFD 42 as committed"
  git push

  # Create PR and merge to default branch

  Troubleshooting

  Problem: Discussion URL not updating

  Check:
  - Is there exactly ONE open PR for your branch?
  - Is the UpdateDiscussionUrl action enabled?
  - Check processor logs for errors

  Problem: PR not auto-created

  Check:
  - Is the CreatePullRequest action enabled?
  - Is your state set to discussion?
  - Is there already a PR (including closed ones) from this branch?

  Problem: State keeps changing back to discussion

  Check:
  - If your PR is open and state is invalid, processor will auto-fix it
  - Valid PR states: discussion, ideation, published, committed, abandoned

  Problem: Can't see my RFD in the frontend

  Check:
  - Visibility: Is it set to public or does your user have GetRfdsAll permission?
  - Use API: POST /rfd/{number}/visibility with {"visibility": "public"}

  Best Practices

  1. Branch Naming: Always use just the 4-digit number (e.g., 0042)
  2. One PR per Branch: Don't create multiple PRs from the same branch
  3. State Management: Update state before significant actions (creating PR, merging)
  4. Discussion Field: Let the processor manage the :discussion: URL - don't edit manually
  5. Commit Messages: Use clear, descriptive commit messages
  6. Review Process: Get stakeholder buy-in before merging to default branch
  7. Visibility: Set RFDs to public if you want them accessible without special permissions

  ---
  This workflow is based on the RFD system architecture and processor actions. The automated processor handles most of the bookkeeping, allowing you to focus on
  the content and discussion.