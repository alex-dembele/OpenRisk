from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
import requests
import os
from dotenv import load_dotenv

load_dotenv()

app = FastAPI(title="OpenRiskOps Dashboard Backend", version="1.0.0")

# CORS for production (restrict allow_origins in prod)
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],  # Change to ["https://your-frontend-domain.com"] in production
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Env variables for URLs and auth (loaded from .env)
OPENRMF_URL = os.getenv('OPENRMF_URL', 'http://openrmf-web:8080')
THEHIVE_URL = os.getenv('THEHIVE_URL', 'http://thehive:9000')
CORTEX_URL = os.getenv('CORTEX_URL', 'http://cortex:9001')
OPENCTI_URL = os.getenv('OPENCTI_URL', 'http://opencti:8080')
THEHIVE_API_KEY = os.getenv('THEHIVE_API_KEY')
OPENCTI_TOKEN = os.getenv('OPENCTI_TOKEN')

@app.get("/health")
def health_check():
    return {"status": "healthy"}

@app.get("/risks")
def get_risks():
    try:
        response = requests.get(f"{OPENRMF_URL}/api/risks", timeout=10)
        response.raise_for_status()
        return response.json()
    except requests.RequestException as e:
        raise HTTPException(status_code=500, detail=f"Error fetching risks: {str(e)}")

@app.get("/incidents")
def get_incidents():
    try:
        headers = {'Authorization': f'Bearer {THEHIVE_API_KEY}'}
        response = requests.get(f"{THEHIVE_URL}/api/v1/query", json={'query': [{'_name': 'listCase'}]}, headers=headers, timeout=10)
        response.raise_for_status()
        return response.json()
    except requests.RequestException as e:
        raise HTTPException(status_code=500, detail=f"Error fetching incidents: {str(e)}")

@app.get("/actions")
def get_actions():
    try:
        response = requests.get(f"{CORTEX_URL}/api/job", timeout=10)  # Adjust the Cortex endpoint if necessary
        response.raise_for_status()
        return response.json()
    except requests.RequestException as e:
        raise HTTPException(status_code=500, detail=f"Error fetching actions: {str(e)}")

@app.get("/threats")
def get_threats():
    try:
        headers = {'Authorization': f'Bearer {OPENCTI_TOKEN}'}
        query = '{ stixCoreObjects { edges { node { id name description } } } }'
        response = requests.post(f"{OPENCTI_URL}/graphql", json={'query': query}, headers=headers, timeout=10)
        response.raise_for_status()
        return response.json()
    except requests.RequestException as e:
        raise HTTPException(status_code=500, detail=f"Error fetching threats: {str(e)}")

@app.get("/reports")
def get_reports():
    try:
        # Example: Retrieve a consolidated report from sync-engine or generate one here
        risks = requests.get(f"{OPENRMF_URL}/api/risks").json()
        incidents = requests.get(f"{THEHIVE_URL}/api/v1/query", json={'query': [{'_name': 'listCase'}]}, headers={'Authorization': f'Bearer {THEHIVE_API_KEY}'}).json()
        return {"risks": risks, "incidents": incidents}
    except requests.RequestException as e:
        raise HTTPException(status_code=500, detail=f"Error generating report: {str(e)}")

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000)