import pytest
from unittest.mock import patch, Mock
from sync import sync_opencti_to_openrmf, sync_thehive_to_openrmf, trigger_cortex_from_thehive, generate_reports  

@patch('requests.post')
@patch('requests.get')
def test_sync_opencti_to_openrmf_success(mock_get, mock_post):
    mock_get.return_value.json.return_value = {
        'data': {'stixCoreObjects': {'edges': [{'node': {'name': 'Test Threat', 'description': 'Mock Desc'}}]}}
    }
    mock_get.return_value.raise_for_status = Mock(return_value=None)
    mock_post.return_value.raise_for_status = Mock(return_value=None)
    
    sync_opencti_to_openrmf()
    
    mock_get.assert_called_once()
    mock_post.assert_called_once_with(expect.anything, json={'name': 'Test Threat', 'description': 'Mock Desc'}, timeout=30)

@patch('requests.get')
def test_sync_opencti_to_openrmf_failure(mock_get):
    mock_get.side_effect = Exception('Mock API Error')
    
    with pytest.raises(Exception):
        sync_opencti_to_openrmf()

@patch('requests.patch')
@patch('requests.post')
def test_sync_thehive_to_openrmf_success(mock_post, mock_patch):
    mock_post.return_value.json.return_value = [{'id': '1', 'status': 'Open'}]
    mock_post.return_value.raise_for_status = Mock(return_value=None)
    mock_patch.return_value.raise_for_status = Mock(return_value=None)
    
    sync_thehive_to_openrmf()
    
    mock_post.assert_called_once()
    mock_patch.assert_called_once_with(expect.anything, json={'status': 'Open'}, timeout=30)

@patch('requests.post')
def test_sync_thehive_to_openrmf_failure(mock_post):
    mock_post.side_effect = Exception('Mock Error')
    
    with pytest.raises(Exception):
        sync_thehive_to_openrmf()


@patch('requests.post')
@patch('requests.post')
def test_trigger_cortex_from_thehive_success(mock_post1, mock_post2):
    mock_post1.return_value.json.return_value = [{'id': '1'}]
    mock_post2.return_value.raise_for_status = Mock(return_value=None)
    
    trigger_cortex_from_thehive()
    
    mock_post1.assert_called_once()
    mock_post2.assert_called()

@patch('requests.post')
@patch('requests.get')
@patch('builtins.open', new_callable=Mock)
def test_generate_reports_success(mock_open, mock_get, mock_post):
    mock_get.return_value.json.return_value = [{'id': '1'}]
    mock_post.return_value.json.return_value = {'data': {'stixCoreObjects': {}}}
    mock_open.return_value.__enter__.return_value.write = Mock()
    
    generate_reports()
    
    mock_open.assert_called_once_with('/app/reports/consolidated_report.json', 'w')
    mock_get.assert_called()
    mock_post.assert_called()

