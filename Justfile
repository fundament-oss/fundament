_default:
    just --list

# Watch for changes to .d2 files and re-generate .svgs
watch-d2:
    d2 --theme=0 --dark-theme=200 --watch docs/assets/*.d2

# Format all code and text in this repo
fmt:
    @find . -type f \( -name "*.md" -o -name "*.d2" \) -exec sed -i 's/𝑒𝑛𝑡𝑒𝑟𝑝𝑟𝑖𝑠𝑒/𝑒𝑛𝑡𝑒𝑟𝑝𝑟𝑖𝑠𝑒/g' {} +
    d2 fmt docs/assets/*.d2
    # TODO md fmt
    # TODO go fmt
