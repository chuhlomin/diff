require.config({ paths: { 'vs': 'https://cdnjs.cloudflare.com/ajax/libs/monaco-editor/0.34.1/min/vs' }});

function xhr(url) {
    var req = null;
    return new Promise(
        function (c, e) {
            req = new XMLHttpRequest();
            req.onreadystatechange = function () {
                if (req._canceled) {
                    return;
                }

                if (req.readyState === 4) {
                    // expected response codes: 200, 201, 204, 404
                    // 0 – when running locally
                    if ((req.status >= 200 && req.status < 300) || req.status == 404 || req.status === 1223 || req.status === 0) {
                        c(req);
                    } else {
                        e(req);
                    }
                    req.onreadystatechange = function () {};
                }
            };

            req.open('GET', url, true);
            req.responseType = '';

            req.send(null);
        },
        function () {
            req._canceled = true;
            req.abort();
        }
    );
}

function loadFiles() {
    var from = document.querySelector('select[name="from"]').value;
    var to = document.querySelector('select[name="to"]').value;
    var files = document.getElementById('files');
    files.src = './files/' + from + '/' + to + '.html';
}

function loadDiff(customEvent) {
    document.getElementById('diff').classList.add('loading');

    originalFile = customEvent.detail.file;
    if (customEvent.detail.oldFile) {
        originalFile = customEvent.detail.oldFile;
    }

    modifiedPath = "./content/" + customEvent.detail.tag2 + "/" + customEvent.detail.file;
    originalPath = "./content/" + customEvent.detail.tag1 + "/" + originalFile;

    Promise.all([xhr(originalPath), xhr(modifiedPath)]).then(function (r) {
        var originalTxt = r[0].responseText;
        var modifiedTxt = r[1].responseText;

        // response status is 0 when running locally
        if (r[0].status == 404) {
            originalTxt = "";
        }
        if (r[1].status == 404) {
            modifiedTxt = "";
        }

        diffEditor.setModel({
            original: monaco.editor.createModel(originalTxt, 'php'),
            modified: monaco.editor.createModel(modifiedTxt, 'php')
        });

        document.getElementById('diff').classList.remove('loading');
    });
}

window.document.addEventListener('loadDiff', loadDiff, false);

var diffEditor;

require(['vs/editor/editor.main'], function () {
    diffEditor = monaco.editor.createDiffEditor(document.querySelector('.diff'), {
        enableSplitViewResizing: false,
        renderSideBySide: true,
        readOnly: true,
        automaticLayout: true,
        scrollBeyondLastLine: false,
        minimap: {
            enabled: false
        }
    });
});

loadFiles();

// listen to window resize events, update the editor layout accordingly
window.addEventListener('resize', function (e) {
    var width = window.innerWidth;
    if (width < 1200) {
        diffEditor.updateOptions({ renderSideBySide: false });
    } else {
        diffEditor.updateOptions({ renderSideBySide: true });
    }
});
