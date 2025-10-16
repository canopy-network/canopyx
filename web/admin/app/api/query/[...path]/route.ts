import { NextRequest } from 'next/server';

const QUERY_SERVICE_URL = process.env.QUERY_SERVICE_URL || 'http://localhost:3001';

async function proxyRequest(request: NextRequest, params: { path: string[] }) {
  const path = params.path.join('/');
  const searchParams = request.nextUrl.searchParams.toString();
  const url = `${QUERY_SERVICE_URL}/${path}${searchParams ? `?${searchParams}` : ''}`;

  const headers = new Headers();

  // Forward relevant headers
  const headersToForward = ['content-type', 'authorization', 'cookie'];
  headersToForward.forEach(header => {
    const value = request.headers.get(header);
    if (value) {
      headers.set(header, value);
    }
  });

  const options: RequestInit = {
    method: request.method,
    headers,
  };

  // Add body for non-GET requests
  if (request.method !== 'GET' && request.method !== 'HEAD') {
    options.body = await request.text();
  }

  const response = await fetch(url, options);

  // Forward the response with all headers
  const responseHeaders = new Headers();
  response.headers.forEach((value, key) => {
    responseHeaders.set(key, value);
  });

  return new Response(response.body, {
    status: response.status,
    statusText: response.statusText,
    headers: responseHeaders,
  });
}

export async function GET(request: NextRequest, context: { params: Promise<{ path: string[] }> }) {
  const params = await context.params;
  return proxyRequest(request, params);
}

export async function POST(request: NextRequest, context: { params: Promise<{ path: string[] }> }) {
  const params = await context.params;
  return proxyRequest(request, params);
}

export async function PUT(request: NextRequest, context: { params: Promise<{ path: string[] }> }) {
  const params = await context.params;
  return proxyRequest(request, params);
}

export async function DELETE(request: NextRequest, context: { params: Promise<{ path: string[] }> }) {
  const params = await context.params;
  return proxyRequest(request, params);
}

export async function PATCH(request: NextRequest, context: { params: Promise<{ path: string[] }> }) {
  const params = await context.params;
  return proxyRequest(request, params);
}