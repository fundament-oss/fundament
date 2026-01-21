const purgecss = require('@fullhuman/postcss-purgecss');

module.exports = {
  plugins: [
    purgecss.default({
      content: ['./src/**/*.{astro,html,js,jsx,ts,tsx,vue,svelte,adoc}'],
      // Safelist patterns - preserve these classes
      safelist: {
        standard: ['fas', 'fa-solid', 'fa', 'far', 'fab'],
        // Use greedy pattern to preserve ALL non-Font-Awesome classes
        greedy: [
          /^(?!fa-).*$/, // Match anything that doesn't start with fa-
          /^fa-(?![\w-]+$)/, // Match fa- followed by anything that's not a simple icon name
        ],
      },
      // Custom extractor to handle Asciidoctor icon syntax
      extractors: [
        {
          extractor: (content) => {
            const classes = [];

            // Extract regular CSS classes (class="foo bar")
            const classMatches = content.match(/class="([^"]*)"/g) || [];
            classMatches.forEach((match) => {
              const extracted = match.match(/class="([^"]*)"/);
              if (extracted) {
                classes.push(...extracted[1].split(/\s+/));
              }
            });

            // Extract Asciidoctor icon syntax: icon:name[]
            const iconMatches = content.match(/icon:([a-z-]+)\[\]/g) || [];
            iconMatches.forEach((match) => {
              const iconName = match.match(/icon:([a-z-]+)\[\]/);
              if (iconName) {
                // Add the Font Awesome class for this icon
                classes.push(`fa-${iconName[1]}`);
              }
            });

            // Also match general words with hyphens
            const wordMatches = content.match(/[\w-]+/g) || [];
            classes.push(...wordMatches);

            return classes;
          },
          extensions: ['astro', 'html', 'js', 'jsx', 'ts', 'tsx', 'vue', 'svelte', 'adoc'],
        },
      ],
    }),
  ],
};
