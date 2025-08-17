/** @type {import('next').NextConfig} */
const nextConfig = {
  distDir: 'out',
  output: 'export',
  basePath: '',
  images: { unoptimized: true },
  experimental: { optimizePackageImports: ['react'] }
};
module.exports = nextConfig;
