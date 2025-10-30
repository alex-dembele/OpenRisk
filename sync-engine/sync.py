import requests
import schedule
import time
from dotenv import load_dotenv
import os

logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')
load_dotenv()

# Env vars for production
OPENCTI_URL = os.getenv('OPENCTI_URL', 'http://opencti:8080')
OPENCTI_TOKEN = os.getenv('OPENCTI_TOKEN')
OPENRMF_URL = os.getenv('OPENRMF_URL', 'http://openrmf:8080')
THEHIVE_URL = os.getenv('THEHIVE_URL', 'http://thehive:9000')
THEHIVE_API_KEY = os.getenv('THEHIVE_API_KEY')
CORTEX_URL = os.getenv('CORTEX_URL', 'http://cortex:9001')

def sync_opencti_to_openrmf():
    # Fetch threats from OpenCTI
    headers = {'Authorization': f'Bearer {OPENCTI_TOKEN}'}
    response = requests.get(f'{OPENCTI_URL}/graphql', json={'query': '{ stixCoreObjects { edges { node { id name } } } }'}, headers=headers)
    threats = response.json().get('data', {}).get('stixCoreObjects', {}).get('edges', [])
    
    # Post to OpenRMF as risks
    for threat in threats:
        risk_data = {'name': threat['node']['name'], 'description': 'From OpenCTI'}
        requests.post(f'{OPENRMF_URL}/api/risks', json=risk_data)

def sync_thehive_to_openrmf():
    # Fetch incidents from TheHive
    headers = {'Authorization': f'Bearer {THEHIVE_API_KEY}'}
    incidents = requests.get(f'{THEHIVE_URL}/api/v1/query', json={'query': [{'_name': 'listCase'}]}, headers=headers).json()
    
    # Update OpenRMF statuses
    for incident in incidents:
        requests.patch(f'{OPENRMF_URL}/api/risks/{incident["id"]}', json={'status': incident['status']})

def trigger_cortex_from_thehive():
    # Example: On new incident, trigger Cortex playbook
    # This would be webhook-based in prod; here simulate with poll
    pass  # Implement polling or use webhooks

def generate_reports():
    # Consolidate data from all
    risks = requests.get(f'{OPENRMF_URL}/api/risks').json()
    incidents = requests.get(f'{THEHIVE_URL}/api/v1/query', json={'query': [{'_name': 'listCase'}]}, headers={'Authorization': f'Bearer {THEHIVE_API_KEY}'}).json()
    threats = requests.get(f'{OPENCTI_URL}/graphql', json={'query': '{ stixCoreObjects { edges { node { id name } } } }'}, headers={'Authorization': f'Bearer {OPENCTI_TOKEN}'}).json()
    # Generate PDF or JSON report; use libraries like reportlab if added
    with open('report.json', 'w') as f:
        f.write({'risks': risks, 'incidents': incidents, 'threats': threats})

# Schedule
schedule.every().day.at("02:00").do(sync_opencti_to_openrmf)
schedule.every().day.at("03:00").do(sync_thehive_to_openrmf)
schedule.every(10).minutes.do(trigger_cortex_from_thehive)
schedule.every().day.at("04:00").do(generate_reports)

while True:
    schedule.run_pending()
    time.sleep(60)