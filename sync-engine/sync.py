import requests
import schedule
import time
from dotenv import load_dotenv
import os
import logging
import json  

logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s', handlers=[logging.FileHandler("sync.log"), logging.StreamHandler()])
load_dotenv()

OPENCTI_URL = os.getenv('OPENCTI_URL', 'http://opencti:8080')
OPENCTI_TOKEN = os.getenv('OPENCTI_TOKEN')
OPENRMF_URL = os.getenv('OPENRMF_URL', 'http://openrmf-web:8080')
THEHIVE_URL = os.getenv('THEHIVE_URL', 'http://thehive:9000')
THEHIVE_API_KEY = os.getenv('THEHIVE_API_KEY')
CORTEX_URL = os.getenv('CORTEX_URL', 'http://cortex:9001')

def sync_opencti_to_openrmf():
    try:
        headers = {'Authorization': f'Bearer {OPENCTI_TOKEN}'}
        response = requests.post(f'{OPENCTI_URL}/graphql', json={'query': '{ stixCoreObjects { edges { node { id name description } } } }'}, headers=headers, timeout=30)
        response.raise_for_status()
        threats = response.json().get('data', {}).get('stixCoreObjects', {}).get('edges', [])
        
        for edge in threats:
            node = edge['node']
            risk_data = {'name': node['name'], 'description': node.get('description', 'From OpenCTI')}
            post_response = requests.post(f'{OPENRMF_URL}/api/risks', json=risk_data, timeout=30)
            post_response.raise_for_status()
        logging.info("Sync OpenCTI to OpenRMF completed successfully")
    except Exception as e:
        logging.error(f"Sync OpenCTI to OpenRMF failed: {str(e)}")

def sync_thehive_to_openrmf():
    try:
        headers = {'Authorization': f'Bearer {THEHIVE_API_KEY}'}
        response = requests.post(f'{THEHIVE_URL}/api/v1/query', json={'query': [{'_name': 'listCase'}]}, headers=headers, timeout=30)
        response.raise_for_status()
        incidents = response.json()
        
        for incident in incidents:
            patch_data = {'status': incident.get('status', 'Unknown')}
            requests.patch(f'{OPENRMF_URL}/api/risks/{incident["id"]}', json=patch_data, timeout=30)
        logging.info("Sync TheHive to OpenRMF completed successfully")
    except Exception as e:
        logging.error(f"Sync TheHive to OpenRMF failed: {str(e)}")

def trigger_cortex_from_thehive():
    try:
        # Polling simulation for new incidents and Cortex triggers
        headers = {'Authorization': f'Bearer {THEHIVE_API_KEY}'}
        incidents = requests.post(f'{THEHIVE_URL}/api/v1/query', json={'query': [{'_name': 'listCase', 'status': 'Open'}]}, headers=headers, timeout=30).json()
        for incident in incidents:
            # Trigger playbook exemple
            requests.post(f'{CORTEX_URL}/api/job', json={'data': incident['id'], 'analyzer': 'example_analyzer'}, timeout=30)
        logging.info("Trigger Cortex from TheHive completed successfully")
    except Exception as e:
        logging.error(f"Trigger Cortex from TheHive failed: {str(e)}")

def generate_reports():
    try:
        risks = requests.get(f'{OPENRMF_URL}/api/risks', timeout=30).json()
        headers_thehive = {'Authorization': f'Bearer {THEHIVE_API_KEY}'}
        incidents = requests.post(f'{THEHIVE_URL}/api/v1/query', json={'query': [{'_name': 'listCase'}]}, headers=headers_thehive, timeout=30).json()
        headers_opencti = {'Authorization': f'Bearer {OPENCTI_TOKEN}'}
        threats = requests.post(f'{OPENCTI_URL}/graphql', json={'query': '{ stixCoreObjects { edges { node { id name description } } } }'}, headers=headers_opencti, timeout=30).json()
        
        report = {
            'risks': risks,
            'incidents': incidents,
            'threats': threats.get('data', {}).get('stixCoreObjects', {})
        }
        with open('/app/reports/consolidated_report.json', 'w') as f:
            json.dump(report, f, indent=4)
        logging.info("Report generation completed successfully")
    except Exception as e:
        logging.error(f"Report generation failed: {str(e)}")

# Scheduling for production
schedule.every().day.at("02:00").do(sync_opencti_to_openrmf)
schedule.every().day.at("03:00").do(sync_thehive_to_openrmf)
schedule.every(10).minutes.do(trigger_cortex_from_thehive)
schedule.every().day.at("04:00").do(generate_reports)

logging.info("Sync Engine started")
while True:
    schedule.run_pending()
    time.sleep(60)