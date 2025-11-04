/** @type {import('next').NextConfig} */
const nextConfig = {
  // distDir: 'out', // Commented out - using default .next for non-static build
  // output: 'export', // Removed - dynamic routes with client components need server capabilities
  basePath: '',
  images: { unoptimized: true },
  experimental: { optimizePackageImports: ['react'] }
};
module.exports = nextConfig;
