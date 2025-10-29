import requests
import schedule
import time
from dotenv import load_dotenv
import os
import logging

logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')
load_dotenv()

# Env vars for production
OPENCTI_URL = os.getenv('OPENCTI_URL', 'http://opencti:8080')
OPENCTI_TOKEN = os.getenv('OPENCTI_TOKEN')
OPENRMF_URL = os.getenv('OPENRMF_URL', 'http://openrmf-web:8080')
THEHIVE_URL = os.getenv('THEHIVE_URL', 'http://thehive:9000')
THEHIVE_API_KEY = os.getenv('THEHIVE_API_KEY')
CORTEX_URL = os.getenv('CORTEX_URL', 'http://cortex:9001')

def sync_opencti_to_openrmf():
    try:
        headers = {'Authorization': f'Bearer {OPENCTI_TOKEN}'}
        response = requests.get(f'{OPENCTI_URL}/graphql', json={'query': '{ stixCoreObjects { edges { node { id name } } } }'}, headers=headers, timeout=30)
        threats = response.json().get('data', {}).get('stixCoreObjects', {}).get('edges', [])
        
        for threat in threats:
            risk_data = {'name': threat['node']['name'], 'description': 'From OpenCTI'}
            requests.post(f'{OPENRMF_URL}/api/risks', json=risk_data, timeout=30)
        logging.info("Sync OpenCTI to OpenRMF completed")
    except Exception as e:
        logging.error(f"Sync failed: {e}")

# Similar try/except for other functions: sync_thehive_to_openrmf, trigger_cortex_from_thehive, generate_reports

schedule.every().day.at("02:00").do(sync_opencti_to_openrmf)
# ... other schedules

while True:
    schedule.run_pending()
    time.sleep(60)