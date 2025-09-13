"""
Context7 MCP Proxy Server

This module implements a simple MCP proxy that forwards all requests to the Context7 MCP server.
It acts as a transparent proxy, forwarding any MCP request and returning the response.
"""

import logging
import os
import traceback

import httpx
from fastapi import FastAPI, Request, HTTPException
from fastapi.responses import JSONResponse

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Context7 configuration
CONTEXT7_BASE_URL = "https://mcp.context7.com/mcp"
CONTEXT7_API_KEY = "ctx7sk-579d7ca0-4646-4a4d-b8b9-a45b321fc845"

# Initialize FastAPI app
app = FastAPI(title="Context7 MCP Proxy", version="1.0.0")
USER_AGENT = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36"

# HTTP client for Context7
http_client = httpx.AsyncClient(
    timeout=30.0,
    headers={
        "Content-Type": "application/json",
        "User-Agent": USER_AGENT
    }
)



@app.middleware("http")
async def log_requests(request: Request, call_next):
    """Log all incoming requests"""
    logger.info(f"Incoming request: {request.method} {request.url.path}")
    response = await call_next(request)
    logger.info(f"Response status: {response.status_code}")
    return response


@app.post("/")
async def proxy_mcp_request(request: Request):
    """
    Proxy all MCP requests to Context7 MCP server
    
    This endpoint forwards any MCP request to the Context7 server and returns the response.
    Context7 requires clients to accept both application/json and text/event-stream.
    """
    request_url = None
    try:
        # Get the request body
        request_data = await request.json()
        logger.info(f"Forwarding MCP request: {request_data.get('method', 'unknown')}")
        
        request_url = request.headers.get("centian-url", CONTEXT7_BASE_URL)
        request_headers = request.headers.mutablecopy()
        logger.info(f"Original request_headers: {request_headers}")
        if "centian-url" in request_headers: 
            del request_headers["centian-url"]
        # Create headers that Context7 expects
        proxy_headers = {
            "Content-Type": "application/json",
            "Accept": "application/json, text/event-stream",
            "User-Agent": USER_AGENT,
            #**request_headers
        }
        if sess_id := request_headers.get("MCP-Session-Id"):
            proxy_headers["mcp-session-id"] = sess_id
        if sess_id := request_headers.get("mcp-session-id"):
            proxy_headers["mcp-session-id"] = sess_id

        # Forward the request to Context7
        logger.info(f"request_url: {request_url}")
        logger.info(f"request_data: {request_data}")
        logger.info(f"proxy_headers: {proxy_headers}")
        response = await http_client.post(
            request_url,
            json=request_data,
            headers=proxy_headers
        )
        if response.status_code == 400:
            logger.error(f"response: {response.json()}")
        response.raise_for_status()
        
        # Check content type to handle different response formats
        content_type = response.headers.get("content-type", "")
        
        if "text/event-stream" in content_type:
            # Handle streaming response - convert SSE to JSON for MCP clients
            logger.info("Received streaming response from Context7, converting to JSON")
            
            # Parse the Server-Sent Events format
            response_text = response.text
            lines = response_text.strip().split('\n')
            
            # Find the data line (usually starts with "data: ")
            json_data = None
            for line in lines:
                if line.startswith("data: "):
                    json_str = line[6:]  # Remove "data: " prefix
                    try:
                        json_data = eval(json_str)  # Parse the JSON string
                        break
                    except:
                        try:
                            import json
                            json_data = json.loads(json_str)
                            break
                        except:
                            continue
            logger.info(f"Got headers for event stream: {response.headers}")
            
            response_headers = {}
            if sess_id := response.headers.get("mcp-session-id"):
                response_headers["mcp-session-id"] = sess_id
            if sess_id := response.headers.get("Mcp-Session-Id"):
                response_headers["mcp-session-id"] = sess_id
            if json_data:
                response = JSONResponse(
                    content=json_data,
                    status_code=response.status_code,
                    headers=response_headers
                )
                logger.info(response.headers)
                return response
            else:
                # Fallback: return the raw text if we can't parse it
                return JSONResponse(content={"error": "Failed to parse streaming response", "raw": response_text}, status_code=500)
        else:
            # Handle JSON response
            logger.info(f"Received JSON response from Context7: {response.status_code}")
            try:
                content_typ = response.headers.get("Content-Type")
                response_data={}
                if content_typ == "application/json":
                    response_data = response.json()
            except Exception as ex:
                logger.error(ex)
            logger.info(f"Got headers: {response.headers}")
            return JSONResponse(
                content=response_data,
                status_code=response.status_code,
                # headers=response.headers or {}
            )
        
    except httpx.HTTPError as e:
        logger.error(f"HTTP error forwarding request to {request_url}: {e}")
        logger.error(traceback.format_exc())
        raise HTTPException(status_code=500, detail=f"Error forwarding request: {str(e)}")
    except Exception as e:
        logger.error(f"Error processing MCP request: {e}")
        logger.error(traceback.format_exc())
        raise HTTPException(
            status_code=500,
            detail=f"Internal server error: {str(e)}"
        )


@app.get("/")
async def root():
    """Health check endpoint"""
    return {
        "service": "Context7 MCP Proxy",
        "status": "running",
        "proxy_target": CONTEXT7_BASE_URL
    }


@app.get("/health")
async def health_check():
    """Health check endpoint"""
    exc = None
    try:
        try:
            # Test connection to Context7
            response = await http_client.get(
                CONTEXT7_BASE_URL,
                timeout=5.0,
                headers={
                    "Content-Type": "text/event-stream",
                    "Accept": "application/json, text/event-stream",
                    "CONTEXT7_API_KEY": "ctx7sk-579d7ca0-4646-4a4d-b8b9-a45b321fc845"
                }
            )
            context7_status = "healthy" if response.status_code == 200 else "unhealthy"
        except Exception as ex:
            logger.info(ex)
            exc = ex
            context7_status = "unreachable"
        
        return {
            "proxy_status": "healthy",
            "context7_status": context7_status,
            "proxy_target": CONTEXT7_BASE_URL,
            "exc": exc,
            # "status": response.status_code,
            #"body": response.json()
        }
    except Exception as ex:
        return {
            "exc": str(ex),
            "trace": traceback.format_exc()
        }


@app.on_event("startup")
async def startup_event():
    """Startup event handler"""
    logger.info("Context7 MCP Proxy Server starting up")
    logger.info(f"Proxy target: {CONTEXT7_BASE_URL}")


@app.on_event("shutdown")
async def shutdown_event():
    """Cleanup resources on shutdown"""
    await http_client.aclose()
    logger.info("Context7 MCP Proxy Server shutdown complete")


if __name__ == "__main__":
    import uvicorn
    
    # Get port from environment or default to 8001
    port = int(os.environ.get("PORT", 8001))
    
    logger.info(f"Starting Context7 MCP Proxy Server on port {port}")
    logger.info(f"Proxying requests to: {CONTEXT7_BASE_URL}")
    
    uvicorn.run(
        "context7_proxy:app",
        host="0.0.0.0",
        port=port,
        reload=True,
        log_level="info"
    )