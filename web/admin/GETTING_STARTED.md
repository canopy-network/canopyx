# Getting Started with the Redesigned CanopyX Admin

## Quick Start

### 1. Start the Backend API (if not running)

Make sure your Go admin server is running on `localhost:3000` (or configured port) with the following endpoints available:
- Authentication: `/api/login`, `/api/logout`
- Chains: `/api/chains`, `/api/chains/status`, etc.

### 2. Configure Environment (Development)

Create `.env.local` in `/web/admin/`:

```bash
NEXT_PUBLIC_API_BASE=http://localhost:3000
```

This tells the frontend where to find the backend API during development.

### 3. Start the Development Server

```bash
cd web/admin
npm run dev
```

The admin interface will be available at **http://localhost:3001** (or next available port).

### 4. Login

Navigate to http://localhost:3001 and you'll be redirected to the login page.

**Default credentials:**
- Username: `admin`
- Password: `admin`

### 5. Explore the Interface

After login, you'll see:
- **Dashboard** - Overview of all your chains with key metrics
- **Chains** - Detailed management of each blockchain indexer

## Main Features

### Dashboard (`/dashboard`)

The dashboard provides a high-level overview:

1. **Stat Cards** showing:
   - Total number of chains (with active/paused breakdown)
   - Total queue backlog across all chains
   - Overall health status
   - Average indexed block height

2. **Chains Overview Table** showing the first 10 chains with their key metrics

3. **Auto-refresh** every 15 seconds to keep data current

### Chains Management (`/chains`)

Complete CRUD operations for your blockchain indexers:

#### View Chains
- Grid of cards, each showing a chain's:
  - Name and ID
  - Health status (healthy/warning/critical/paused)
  - Key metrics (indexed height, head, backlog)
  - Queue details (workflow, activity, pollers, age)
- Real-time updates every 15 seconds

#### Create Chain
1. Click **"Add Chain"** button
2. Fill in the form:
   - **Chain ID**: Unique identifier (e.g., `ethereum_mainnet`)
   - **Chain Name**: Display name (e.g., `Ethereum Mainnet`)
   - **Container Image**: Docker image (e.g., `ghcr.io/myorg/indexer:latest`)
   - **RPC Endpoints**: One per line (e.g., Alchemy, Infura URLs)
   - **Min/Max Replicas**: Worker scaling configuration
   - **Notes**: Optional description
3. Click **"Create Chain"**

#### Edit Chain
1. Hover over a chain card and click the **edit icon** (top-right)
2. Update:
   - Container image
   - Min/Max replicas
   - Notes
3. Click **"Save Changes"**

#### Chain Operations

Each chain has quick action buttons:

- **Pause/Resume**: Stop or start indexing
- **Head Scan**: Trigger a scan to find the latest block
- **Gap Scan**: Trigger a scan to find missing blocks
- **Reindex**: Re-index specific blocks
  - Single height: Enter one block number
  - Range: Enter start and end block numbers (max 500)

## Tips & Tricks

### Navigation
- Use the **sidebar** to switch between Dashboard and Chains
- Click your **user avatar** at the bottom to see account info
- Click **"Sign Out"** to logout and clear your session

### Refresh Data
- Click the **Refresh button** on any page to manually update data
- Data auto-refreshes every 15 seconds

### Status Indicators
- **Green badge**: Healthy, queue backlog is normal
- **Amber badge**: Warning, queue backlog is elevated
- **Red badge**: Critical, queue backlog is very high
- **Amber "Paused" badge**: Chain is manually paused

### Toast Notifications
- Success messages appear in **blue** in the top-right
- Error messages appear in **red** in the top-right
- Click **"Close"** or wait 5 seconds for auto-dismiss

### Keyboard Shortcuts
- Press **Enter** in the login form to submit
- Press **Escape** to close any open modal
- Use **Tab** to navigate through form fields

## Common Tasks

### Task: Add a New Chain

1. Go to **Chains** page
2. Click **"Add Chain"**
3. Fill in required fields (marked with *)
4. Add at least one RPC endpoint
5. Adjust replicas if needed (default 1-3)
6. Click **"Create Chain"**
7. Wait for success notification
8. New chain appears in the grid

### Task: Monitor Queue Health

1. Go to **Dashboard**
2. Check the **"Health Status"** card
   - Shows how many chains are healthy/warning/critical
3. For detailed view, go to **Chains**
4. Look at each chain's status badge
5. Click into a chain to see detailed queue metrics

### Task: Reindex Missing Blocks

1. Go to **Chains**
2. Find the chain that needs reindexing
3. Click **"Reindex"** button
4. Choose either:
   - **Single height**: Enter one block number
   - **Range**: Enter start and end (max 500 blocks)
5. Click **"Queue Reindex"**
6. Check notification for how many blocks were queued

### Task: Update Container Image

1. Go to **Chains**
2. Hover over the chain card
3. Click the **edit icon** (pencil)
4. Update the **"Container Image"** field
5. Click **"Save Changes"**
6. Workers will use new image on next deployment

### Task: Pause Indexing

1. Go to **Chains**
2. Find the chain to pause
3. Click **"Pause"** button
4. Status badge changes to "Paused"
5. To resume: click **"Resume"** button

## Troubleshooting

### Can't Login
- Check that backend API is running
- Verify `NEXT_PUBLIC_API_BASE` in `.env.local`
- Check console for network errors
- Try default credentials: `admin` / `admin`

### Redirect Loop
- Clear browser cookies
- Check that backend `/api/login` endpoint is working
- Verify session cookies are being set

### Data Not Loading
- Check that backend API is running
- Open browser DevTools → Network tab
- Look for failed API requests
- Check that endpoints return proper JSON

### Styling Looks Wrong
- Run `npm run build` to check for CSS errors
- Clear browser cache
- Check that Tailwind is compiling correctly

### Auto-refresh Not Working
- Check browser console for errors
- Verify that `/api/chains/status` endpoint is working
- Page must be in focus for refresh to work

## Production Deployment

### Build for Production

```bash
cd web/admin
npm run build
```

This creates an optimized production build in `.next/`.

### Environment Variables

For production, you can:

**Option 1: Same-origin (recommended)**
- Don't set `NEXT_PUBLIC_API_BASE`
- Serve the frontend from the same domain as the backend
- All API requests will be relative

**Option 2: Different origin**
```bash
NEXT_PUBLIC_API_BASE=https://api.yourdomain.com
```

### Serve Static Export

Since this is a static export, you can:
1. Export the static files: `npm run build`
2. Serve the `out/` directory with any web server
3. Configure your Go backend to serve these static files

### Security Considerations

- Use HTTPS in production
- Configure proper CORS headers on backend
- Use secure session cookies (HttpOnly, Secure, SameSite)
- Implement rate limiting on login endpoint
- Consider adding 2FA for admin access
- Rotate default admin credentials
- Add audit logging for admin actions

## Architecture Overview

```
┌─────────────────────────────────────────┐
│         User's Browser                   │
│  ┌───────────────────────────────────┐  │
│  │   Next.js Frontend (React)        │  │
│  │   - Auth Context                  │  │
│  │   - Dashboard                     │  │
│  │   - Chains Management             │  │
│  │   - API Client (fetch)            │  │
│  └───────────┬───────────────────────┘  │
└─────────────┼───────────────────────────┘
              │ HTTP + Cookies (session)
              ↓
┌─────────────────────────────────────────┐
│      Go Admin Server (Backend)          │
│  ┌───────────────────────────────────┐  │
│  │   /api/login                      │  │
│  │   /api/logout                     │  │
│  │   /api/chains (CRUD)              │  │
│  │   /api/chains/status              │  │
│  │   /api/chains/{id}/headscan       │  │
│  │   /api/chains/{id}/gapscan        │  │
│  │   /api/chains/{id}/reindex        │  │
│  └───────────┬───────────────────────┘  │
└─────────────┼───────────────────────────┘
              │
              ↓
┌─────────────────────────────────────────┐
│      Database & Temporal                │
│  - Chain configurations                 │
│  - Queue metrics                        │
│  - Reindex history                      │
└─────────────────────────────────────────┘
```

## Support & Feedback

### Reporting Issues
- Check browser console for errors
- Check network tab for failed requests
- Note the exact steps to reproduce
- Include screenshots if possible

### Feature Requests
- Describe the use case
- Explain the expected behavior
- Consider if it fits the current design

## Next Steps

Now that you're familiar with the basics:

1. **Customize** the color scheme in `app/globals.css`
2. **Add monitoring** - integrate with your observability tools
3. **Extend functionality** - add new features as needed
4. **Improve security** - add 2FA, audit logs, etc.
5. **Optimize performance** - add caching, pagination, etc.

## Resources

- **Next.js Docs**: https://nextjs.org/docs
- **Tailwind CSS**: https://tailwindcss.com/docs
- **Radix UI**: https://www.radix-ui.com/primitives/docs/overview/introduction
- **React Hooks**: https://react.dev/reference/react

---

**Congratulations!** You now have a modern, production-ready admin interface for CanopyX. Enjoy managing your blockchain indexers with style! 🚀